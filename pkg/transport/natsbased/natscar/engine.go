package natscar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/refunc/refunc/pkg/utils"

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

type taskDoneFunc func() (reply string, expired bool)

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

	eng.fn = fn

	klog.V(2).Infof("(natscar) svc endpoint: %s", svcEndpoint)

	// apply envs
	for k, v := range fn.Spec.Runtime.Envs {
		if v != "" {
			// try to expand env
			if strings.HasPrefix(v, "$") {
				v = os.ExpandEnv(v)
			}
			os.Setenv(k, v)
		}
	}

	os.Setenv("REFUNC_ENV", "cluster")
	os.Setenv("REFUNC_NAMESPACE", fn.Namespace)
	os.Setenv("REFUNC_NAME", fn.Name)
	os.Setenv("REFUNC_HASH", fn.Spec.Hash)

	if fn.Spec.Runtime.Credentials.Token != "" {
		os.Setenv("REFUNC_TOKEN", fn.Spec.Runtime.Credentials.Token)
	}

	os.Setenv("REFUNC_ACCESS_KEY", fn.Spec.Runtime.Credentials.AccessKey)
	os.Setenv("REFUNC_SECRET_KEY", fn.Spec.Runtime.Credentials.SecretKey)

	os.Setenv("REFUNC_MINIO_SCOPE", fn.Spec.Runtime.Permissions.Scope)
	os.Setenv("REFUNC_MAX_TIMEOUT", fmt.Sprintf("%d", fn.Spec.Runtime.Timeout))

	// reload envs
	env.RefreshEnvs()

	// connect to nats
	conn, err := env.NewNatsConn(nats.Name(fn.Namespace + "/" + fn.Name))
	if err != nil {
		return fmt.Errorf("failed to connect to nats %s, %v", env.GlobalNATSEndpoint, err)
	}
	eng.conn = conn

	eng.ctx, eng.cancel = context.WithCancel(ctx)

	// setup handler to respond request
	sub, err := conn.QueueSubscribe(svcEndpoint, "_svc_", func(msg *nats.Msg) {
		if msg.Reply == "" {
			klog.Errorf("(natscar) got invalid request, empty reply")
			return
		}
		msgReply := msg.Reply

		// verify request
		var req *messages.InvokeRequest
		err := json.Unmarshal(msg.Data, &req)
		if err != nil {
			eng.replyError(msgReply, err)
			return
		}

		var (
			reqCtx context.Context
			cancel context.CancelFunc
		)

		// support potential long running task
		if req.Deadline.IsZero() {
			reqCtx, cancel = eng.ctx, func() {}
		} else {
			reqCtx, cancel = context.WithDeadline(eng.ctx, req.Deadline)
		}

		// write reply as rquest id
		rid := utils.GenID([]byte(msgReply))
		req.RequestID = rid

		// create session
		var once sync.Once
		doneFunc := taskDoneFunc(func() (reply string, expired bool) {
			finished := reqCtx.Err() != nil
			once.Do(func() {
				// cancel task & cleanup
				cancel()
				eng.sessions.Delete(rid)
			})
			return msgReply, finished
		})

		eng.sessions.Store(req.RequestID, doneFunc)

		if !req.Deadline.IsZero() {
			go func() {
				<-reqCtx.Done()
				doneFunc()
			}()
		}

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
		defer klog.V(2).Infof("(natscar) %s exited", fn.Name)
		defer conn.Close()
		defer conn.Flush()
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
		reply, expired := doneFunc()
		if expired {
			klog.Warningf("(natscar) request expired %q", rid)
			return invalidRequestIDErr(rid)
		}
		eng.publish(reply, messages.MustFromObject(&messages.Action{
			Type: messages.Response,
			Payload: messages.MustFromObject(&messages.InvokeResponse{
				Payload: body,
				Error:   messages.GetErrorMessage(err),
			}),
		}))
		return nil
	}
	klog.Warningf("(natscar) cannot find request %q", rid)
	return invalidRequestIDErr(rid)
}

func (eng *engine) ReportInitError(err error) {
	klog.Infof("(natscar) ReportInitError: %v", err)
}

func (eng *engine) ReportReady() {
	// explicity send cry message to notify that we'r ready
	eng.publish(eng.fn.Spec.Runtime.Envs["REFUNC_CRY_ENDPOINT"], nil)
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
