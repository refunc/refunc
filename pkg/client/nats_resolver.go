package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
	observer "github.com/refunc/go-observer"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

// NewNatsResolver returns new task resolver using NATS based RPC
func NewNatsResolver(ctx context.Context, nc *nats.Conn, endpoint string, request *messages.InvokeRequest) (TaskResolver, error) {

	reqID := request.RequestID
	name := fmt.Sprintf("%s<%s>", endpoint, func() string {
		if len(reqID) > 32 {
			return reqID[:len(reqID)-25]
		}
		if len(reqID) > 7 {
			return reqID[:7]
		}
		return reqID
	}())

	SetReqeustDeadline(ctx, request)

	ctx, cancel := context.WithDeadline(ctx, request.Deadline)

	logger := GetLogger(ctx)

	// prevent tasker done with deadline, tasker only done with SetResult
	trCtx, trCancel := context.WithCancel(context.Background())

	tr := &natsResolver{
		Logger: logger,
		hash:   reqID,
		name:   name,
		t0:     time.Now(),
		ctx:    trCtx,
		cancel: func() {
			trCancel()
			cancel()
		},
		msgSrc: observer.NewProperty(nil),
		logSrc: observer.NewProperty(nil),
	}

	var logSubs interface {
		Unsubscribe() error
	}
	if IsLoggingEnabled(ctx) {
		logger.Infof("%s start receiving logs", tr.name)
		logsEndpoint := fmt.Sprintf("_refunc.forwardlogs.%s.%s", strings.Replace(endpoint, "/", ".", -1), nuid.Next())
		subs, err := nc.Subscribe(logsEndpoint, func(msg *nats.Msg) {
			tr.UpdateLog(msg.Data)
		})
		if err != nil {
			tr.SetResult(nil, err)
			return tr, nil
		}

		if request.Options == nil {
			request.Options = make(map[string]interface{})
		}
		request.Options["logEndpoint"] = logsEndpoint
		logSubs = subs
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			tr.res.t1 = time.Now()
			msg := fmt.Sprintf("%s done in %v, emit %d msgs", tr.Name(), tr.res.t1.Sub(tr.t0), atomic.LoadUint64(&tr.res.nemit))
			plsz := atomic.LoadUint64(&tr.outs.plsz)
			if plsz > 0 {
				msg = fmt.Sprintf("%s, pub %d msg, %s produced", msg, tr.outs.pmsg, utils.ByteSize(plsz))
			}
			if tr.res.err != nil {
				msg = fmt.Sprintf("%s, with error, %v", msg, tr.res.err)
			}
			tr.Info(msg)
		}()

		defer func() {
			if re := recover(); re != nil {
				utils.LogTraceback(re, 5, tr.Logger)
				err = fmt.Errorf("h: %v", re)
			}
		}()

		defer func() {
			if logSubs != nil {
				logSubs.Unsubscribe() // nolint:errcheck
			}
		}()

		msg, err := nc.RequestWithContext(ctx, "refunc."+strings.Replace(endpoint, "/", ".", -1), data)
		if err != nil {
			tr.SetResult(nil, err)
			return
		}
		ParseAction(msg.Data, tr)
	}()

	return tr, nil
}

type natsResolver struct {
	Logger

	hash string
	name string
	t0   time.Time

	ctx    context.Context
	cancel context.CancelFunc

	msgSrc observer.Property
	logSrc observer.Property

	outs struct {
		plsz uint64
		pmsg uint64
	}

	res struct {
		sync.Once

		data []byte

		t1    time.Time
		nemit uint64
		err   error
	}
}

// ID unique string to identify this task
func (tr *natsResolver) ID() string {
	return tr.hash
}

// Name returns a human readable string,
// could be same with ID
func (tr *natsResolver) Name() string {
	return tr.name
}

func (tr *natsResolver) Done() <-chan struct{} {
	return tr.ctx.Done()
}

func (tr *natsResolver) Cancel() {
	if tr != nil && tr.cancel != nil {
		tr.cancel()
	}
}

func (tr *natsResolver) LogObserver() observer.Stream {
	return tr.logSrc.Observe()
}

func (tr *natsResolver) MsgObserver() observer.Stream {
	return tr.msgSrc.Observe()
}

// Result returns the ouput of task once exited
func (tr *natsResolver) Result() ([]byte, error) {
	return tr.res.data, tr.res.err
}

// StatJSON returns statistic info of running task
func (tr *natsResolver) StatJSON() string {
	if tr == nil {
		return ""
	}

	bts, _ := json.Marshal(struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"createdAt"`
		UpTime    string    `json:"uptime"`
		Received  uint64    `json:"received"`
		Published uint64    `json:"published,omitempty"`
	}{
		tr.ID(),
		tr.t0,
		time.Since(tr.t0).String(),
		atomic.LoadUint64(&tr.res.nemit),
		atomic.LoadUint64(&tr.outs.pmsg),
	})
	return string(bts)
}

func (tr *natsResolver) SetResult(data []byte, err error) {
	if tr == nil {
		return
	}

	tr.res.Do(func() {
		if len(data) > 0 {
			tr.res.data = bytes.TrimSuffix(data, messages.TokenCRLF)
			// recorde payload size
			atomic.AddUint64(&tr.outs.plsz, uint64(len(tr.res.data)))
		}
		tr.res.err = err
		if tr.cancel != nil {
			tr.cancel()
		}
		if tr.res.err != nil {
			tr.Infof("(tr) %s on error, %v", tr.Name(), tr.res.err)
		}
	})
}

func (tr *natsResolver) UpdateLog(line []byte) {
	tr.logSrc.Update(unquote(line))
}
