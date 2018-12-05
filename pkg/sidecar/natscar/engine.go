package natscar

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog"

	nats "github.com/nats-io/go-nats"
	observer "github.com/refunc/go-observer"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/sidecar"
)

type engine struct {
	sync.Mutex

	fn *types.Function

	actions observer.Property
	stream  observer.Stream

	ctx    context.Context
	cancel context.CancelFunc

	// track tasks by request id
	sessions sync.Map

	conn *nats.Conn
}

type taskDoneFunc func() (expired bool)

// NewEngine returns a nats based engine
func NewEngine() sidecar.Engine {
	eng := &engine{
		actions: observer.NewProperty(nil),
	}
	eng.stream = eng.actions.Observe()
	return eng
}

func (eng *engine) Init(ctx context.Context, fn *types.Function) error {
	var (
		cryEndpoint    = fn.Spec.Runtime.Envs["REFUNC_CRY_ENDPOINT"]
		svcEndpoint    = fn.Spec.Runtime.Envs["REFUNC_SVC_ENDPOINT"]
		tapEndpoint    = fn.Spec.Runtime.Envs["REFUNC_TAP_ENDPOINT"]
		crysvcEndpoint = fn.Spec.Runtime.Envs["REFUNC_CRY_SVC_ENDPOINT"]
		// logEndpoint    = fn.Spec.Runtime.Envs["REFUNC_LOG_ENDPOINT"]
	)

	klog.V(2).Infof("(natscar) cry endpoint: %s", cryEndpoint)
	klog.V(2).Infof("(natscar) svc endpoint: %s", svcEndpoint)

	// connect to nats
	conn, err := env.NewNatsConn(nats.Name(fn.Namespace + "/" + fn.Name))
	if err != nil {
		return fmt.Errorf("failed to connect to nats %s, %v", env.GlobalNATSEndpoint, err)
	}

	eng.ctx, eng.cancel = context.WithCancel(ctx)

	// setup handler to respond request
	sub, err := conn.QueueSubscribe(svcEndpoint, "_svc_", func(msg *nats.Msg) {
		if msg.Reply == "" {
			klog.Errorf("(natscar) got invalid request, empty reply")
			return
		}
		reply := msg.Reply

		// verify request
		var req *messages.InvokeRequest
		err := json.Unmarshal(msg.Data, &req)
		if err != nil {
			eng.replyError(reply, err)
			return
		}

		// write reply as rquest id
		req.RequestID = reply

		// create session
		ctx, cancel := context.WithDeadline(eng.ctx, req.Deadline)

		var once sync.Once
		doneFunc := func() (expired bool) {
			finished := ctx.Err() != nil
			once.Do(func() {
				// cancel task & cleanup
				cancel()
				eng.sessions.Delete(reply)
			})
			return finished
		}

		eng.sessions.Store(reply, doneFunc)

		go func() {
			<-ctx.Done()
			doneFunc()
		}()

		// enqueue
		eng.actions.Update(req)
	})

	if err != nil {
		return err
	}

	// setup ping/pong service to respond cry request
	crySubs, err := conn.QueueSubscribe(crysvcEndpoint, "_svc_", func(msg *nats.Msg) {
		reply := msg.Reply
		if reply == "" {
			reply = cryEndpoint
		}
		conn.Publish(reply, nil)
	})
	if err != nil {
		sub.Unsubscribe()
		return err
	}

	// start ioloop
	go func() {
		defer sub.Unsubscribe()
		defer crySubs.Unsubscribe()

		klog.V(2).Infof("(natscar) %s started", fn.Name)
		tapTicker := time.NewTicker(2 * time.Second)
		defer tapTicker.Stop()
		for {
			select {
			case <-eng.ctx.Done():
				// wait until we are requested to leave
				return
			case <-tapTicker.C:
				var hasTasks bool
				eng.sessions.Range(func(key, value interface{}) bool {
					hasTasks = true
					return false
				})
				if hasTasks {
					eng.publish(tapEndpoint, nil)
				}
			}
		}
	}()

	return nil
}

func (eng *engine) NextC() <-chan struct{} {
	eng.Lock()
	defer eng.Unlock()
	return eng.stream.Changes()
}

func (eng *engine) InvokeRequest() *messages.InvokeRequest {
	eng.Lock()
	defer eng.Unlock()
	if eng.stream.HasNext() {
		return eng.stream.Next().(*messages.InvokeRequest)
	}
	return nil
}

func (eng *engine) SetResult(rid string, body []byte, err error) error {
	if v, ok := eng.sessions.Load(rid); ok {
		doneFunc := v.(taskDoneFunc)
		if expired := doneFunc(); expired {
			return invalidRequestIDErr(rid)
		}
		eng.publish(rid, messages.MustFromObject(&messages.Action{
			Type: messages.Response,
			Payload: messages.MustFromObject(&messages.InvokeResponse{
				Payload: body,
				Error:   messages.GetErrorMessage(err),
			}),
		}))
		return nil
	}
	return invalidRequestIDErr(rid)
}

func (eng *engine) ReportInitError(err error) {
	klog.Infof("(natscar) ReportInitError: %v", err)
}

func (eng *engine) ReportReady() {
	// explicity send cry message to notify that we'r ready
	eng.publish(eng.fn.Spec.Runtime.Envs["REFUNC_CRY_SVC_ENDPOINT"], nil)
}

func (eng *engine) ReportExiting() {
	klog.Infoln("(natscar) ReportExiting")
	if eng.cancel != nil {
		eng.cancel()
	}
}

func (eng *engine) replyError(reply string, err error) {
	klog.V(3).Infof("(sidecar) request on error, %v", err)
	if err := eng.conn.Publish(reply, messages.GetErrActionBytes(err)); err != nil {
		klog.Errorf("(sidecar) failed to reply error to %q, %v", reply, err)
		return
	}
}

func (eng *engine) publish(endpoint string, bts []byte) {
	if err := eng.conn.Publish(endpoint, bts); err != nil {
		klog.Errorf("(natscar) publish to %s failed, %v", endpoint, err)
	}
}

func invalidRequestIDErr(rid string) error {
	return messages.ErrorMessage{
		Type:    "InvalidRequestID",
		Message: fmt.Sprintf("Invalid request ID: %q", rid),
	}
}
