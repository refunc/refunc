package natsbased

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/refunc/refunc/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	nats "github.com/nats-io/nats.go"
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/builtins"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/operators"
	"github.com/refunc/refunc/pkg/utils"
)

var (
	errInvalidRequest = errors.New("Invalid request topic")
)

const codeLaunchBias = 10 * time.Second

func (nh *natsHandler) Start(ctx context.Context, operator operators.Interface) {
	nh.operator = operator
	nh.ctx, nh.cancel = context.WithCancel(ctx)
	defer nh.cancel()

	ns := nh.operator.GetNamespace()
	if ns == "" {
		ns = "*"
	}
	conn := nh.natsConn

	reqSubs, err := conn.QueueSubscribe("refunc."+ns+".*", "_req_", nh.onRequest)
	if err != nil {
		panic(err)
	}
	defer reqSubs.Unsubscribe()

	metaSubs, err := conn.QueueSubscribe("refunc."+ns+".*._meta", "_req_", nh.onRequest)
	if err != nil {
		panic(err)
	}
	defer metaSubs.Unsubscribe()

	crySubs, err := conn.Subscribe("_refunc._cry_.*", nh.onCry)
	if err != nil {
		panic(err)
	}
	defer crySubs.Unsubscribe()

	tapSubs, err := conn.QueueSubscribe("_refunc._tap_.*", "_tap_", nh.onTap)
	if err != nil {
		panic(err)
	}
	defer tapSubs.Unsubscribe()

	<-nh.ctx.Done()
}

func (nh *natsHandler) onRequest(msg *nats.Msg) {
	if msg.Reply == "" {
		// only request is accepted
		return
	}
	klog.V(4).Infof("(nh) new request from %q", msg.Subject)

	ns, name, path, err := nh.splitTopic(msg.Subject)
	if err != nil {
		nh.replyError(msg.Subject, msg.Reply, err)
		return
	}

	// handle builtins
	if ns == "builtins" {
		go func() {
			defer func() {
				if re := recover(); re != nil {
					utils.LogTraceback(re, 5, klog.V(1))
				}
			}()
			builtins.HandleBuiltins(name, msg.Data, func(res []byte, err error) {
				if err != nil {
					nh.replyError(msg.Subject, msg.Reply, err)
					return
				}
				if err := nh.natsConn.Publish(msg.Reply, res); err != nil {
					klog.Errorf("(nh) buildin %q failed to reply, %v", name, err)
				}
			})
		}()
		return
	}

	trigger, err := nh.operator.TriggerForEndpoint(ns + "/" + name)
	if err != nil {
		nh.replyError(msg.Subject, msg.Reply, err)
		return
	}

	fndef, err := nh.operator.ResolveFuncdef(trigger)
	if err != nil {
		nh.replyError(msg.Subject, msg.Reply, err)
		return
	}

	// dispatch messages
	switch path {
	case "_meta":
		// TODO (bin): maybe using annotations
		nh.replyMeta(msg.Reply, fndef)
	default:
		go nh.forwardRequest(msg, fndef, trigger)
	}
}

func (nh *natsHandler) onCry(msg *nats.Msg) {
	val, has := nh.cryMap.Load(msg.Subject)
	if !has {
		return
	}
	_, sig := val.(crySigFn)()
	sig(true)
}

func (nh *natsHandler) onTap(msg *nats.Msg) {
	subject := msg.Subject[len("_refunc._tap_."):]
	// verify
	if splitted := strings.SplitN(subject, "/", 2); len(splitted) != 2 {
		klog.Errorf("(nh) malformed tap message on %s", msg.Subject)
		return
	}
	nh.operator.Tap(subject)
}

func (nh *natsHandler) replyMeta(to string, fndef *rfv1beta3.Funcdef) {
	if err := nh.natsConn.Publish(to, []byte{'{', '}'}); err != nil {
		nh.replyError(fmt.Sprintf("refunc.%s.%s._meta", fndef.Namespace, fndef.Name), to, err)
	}
	return
}

func (nh *natsHandler) forwardRequest(msg *nats.Msg, fndef *rfv1beta3.Funcdef, trigger *rfv1beta3.Trigger) {
	defer func() {
		if re := recover(); re != nil {
			utils.LogTraceback(re, 5, klog.V(1))
		}
	}()

	t0 := time.Now()
	fninst, err := nh.operator.GetFuncInstance(trigger)
	if err != nil {
		nh.replyError(msg.Subject, msg.Reply, err)
		return
	}

	// parse job max timeout for a running job
	var timeout = messages.DefaultJobTimeout
	if fndef.Spec.Runtime != nil && fndef.Spec.Runtime.Timeout > 0 {
		timeout = time.Second*time.Duration(fndef.Spec.Runtime.Timeout) + codeLaunchBias
	}

	ctx, cancel := context.WithTimeout(nh.ctx, timeout)
	defer cancel()

	cryEndpoint := fninst.CryingEndpoint()
	reqEndpoint := fninst.ServiceEndpoint()
	conn := nh.natsConn

	// verify request
	data := msg.Data
	var req messages.InvokeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		nh.replyError(msg.Subject, msg.Reply, err)
		return
	}

	if req.Deadline.IsZero() {
		// enforce deadline
		client.SetReqeustDeadline(ctx, &req)
	}
	if !(fninst.Status.Active > 0) {
		req.Deadline = time.Time{}
		client.SetReqeustDeadline(ctx, &req) // reset cry req deadline with codeLaunchBias
		if data, err = json.Marshal(req); err != nil {
			nh.replyError(msg.Subject, msg.Reply, err)
		}
	}

	// forwarding request
	forwardReq := func() {
		// send request
		if err := conn.PublishMsg(&nats.Msg{
			Subject: reqEndpoint,
			Reply:   msg.Reply,
			Data:    data,
		}); err != nil {
			nh.replyError(msg.Subject, msg.Reply, err)
			return
		}

		if dt := time.Now().Sub(t0); dt > 200*time.Millisecond {
			klog.Warningf("(nh) forwarded one slow request for %q using %v", fninst.Name, dt)
		}
	}

	if fninst.Status.Active > 0 {
		// funcinst is active and has backends forward immediately
		klog.V(4).Infof("(nh) forwarding request to %q", fninst.Name)
		forwardReq()
		return
	}

	klog.V(3).Infof("(nh) forward request when %q is online", fninst.Name)

	cryReqEp := fninst.CryServiceEndpoint()
	val, _ := nh.cryMap.LoadOrStore(cryEndpoint, nh.newCrySig(cryEndpoint, func() { conn.Publish(cryReqEp, nil) }))
	onlineC, sig := val.(crySigFn)()
	defer sig(false) // ensure no resource leak

	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			nh.replyError(msg.Subject, msg.Reply, ctx.Err())
			return
		case <-onlineC:
			forwardReq()
			return
		case <-ticker.C:
			fninst, err = nh.operator.GetFuncInstance(trigger)
			if err != nil {
				nh.replyError(msg.Subject, msg.Reply, err)
				return
			}
			// check if something goes wrong
			for _, cond := range fninst.Status.Conditions {
				switch cond.Type {
				case rfv1beta3.FuncinstInactive:
					if fninst.Status.IsInactiveCondition() {
						nh.replyError(msg.Subject, msg.Reply, errors.New(cond.Message))
						return
					}
				case rfv1beta3.FuncinstPending:
					if cond.Status == corev1.ConditionTrue && cond.Reason == "XenvNotResolved" {
						nh.replyError(msg.Subject, msg.Reply, errors.New(cond.Message))
						return
					}
				case rfv1beta3.FuncinstActive:
					if cond.Status == corev1.ConditionFalse {
						switch cond.Reason {
						case "ReplicasetNotReady":
							nh.replyError(msg.Subject, msg.Reply, errors.New(cond.Message))
							return
						}
					}
				}
			}
		}
	}
}

func (nh *natsHandler) replyError(from, to string, err error) {
	if to == "" {
		return
	}
	klog.V(3).Infof("(nh) %q on error, %v", from, err)
	if err := nh.natsConn.Publish(to, messages.GetErrActionBytes(err)); err != nil {
		klog.Errorf("(nh) failed to reply error to %q, %v", to, err)
		return
	}
}

func (nh *natsHandler) splitTopic(topic string) (ns, name, path string, err error) {
	// trim prefix refunc.
	topic = topic[7:]
	parts := strings.SplitN(topic, ".", 3)
	switch len(parts) {
	case 2:
		ns, name = parts[0], parts[1]
		return
	case 3:
		ns, name, path = parts[0], parts[1], parts[2]
		return
	}
	err = errInvalidRequest
	return
}

type crySigFn func() (onlineC <-chan struct{}, sig func(bool))

func (nh *natsHandler) newCrySig(key string, poke func()) crySigFn {
	var (
		ch       chan struct{}
		sigFn    func(bool)
		initOnce sync.Once
		// refrence count,
		// ensure ch is closed correctly
		// when multiple goroutines are waiting for crying.
		refCnt int32
	)

	return func() (onlineC <-chan struct{}, sig func(bool)) {
		// always increase ref cnt
		atomic.AddInt32(&refCnt, 1)
		initOnce.Do(func() {
			ch = make(chan struct{})

			go func() {
				probeTicker := time.NewTicker(47 * time.Millisecond)
				defer probeTicker.Stop()
				select {
				case <-ch:
					return
				case <-probeTicker.C:
					poke()
				}
			}()

			var sigOnce sync.Once
			sigFn = func(fromRemote bool) {
				// close ch only when refCnt==0 (which means no forwading process is waiting on)
				// or fromRemote is true(in the case, we got cry from funcinst)
				if atomic.AddInt32(&refCnt, -1) == 0 || fromRemote {
					sigOnce.Do(func() {
						nh.cryMap.Delete(key)
						if fromRemote {
							klog.V(4).Infof("(nh) got cry from %q", key)
						}
						close(ch)
					})
				}
			}
		})
		return ch, sigFn
	}
}
