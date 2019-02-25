package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	observer "github.com/refunc/go-observer"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

// common errors
var (
	ErrEmptyResponse   = errors.New("task: empty response")
	ErrTaskStopped     = errors.New("task: stopped")
	ErrNilOutputStream = errors.New("task: output stream is nil")
	ErrNilInputStream  = errors.New("task: input stream is nil")
)

// Task is modeled like a process with stdin, stdout, stderr
type Task interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
}

// TaskFactory used to build task
type TaskFactory func(ctx context.Context) (Task, error)

// SimpleResolver provides a simple implementation of TaskResolver interface.
// It will polling result from input stream and handle result or messages
type SimpleResolver struct {
	// OutputStream is used to recive actions from upstream
	InputStream io.Reader
	// OutputStream is used to send actions to upstream (optional)
	OutputStream io.Writer

	id   string
	name string

	msgSrc observer.Property
	logSrc observer.Property

	ctx    context.Context
	cancel context.CancelFunc

	task struct {
		factory TaskFactory
		inst    Task
	}

	startOnce struct {
		sync.Once
		started bool

		t0 time.Time
	}

	outs struct {
		startC chan struct{}

		plsz uint64
		pmsg uint64
	}

	res struct {
		sync.Once

		done func(Task, []byte, error)
		data []byte

		t1    time.Time
		nemit uint64
		err   error
	}
}

// NewSimpleResolver creates a SimpleResolver with given ID & name
func NewSimpleResolver(ID, name string) *SimpleResolver {
	return &SimpleResolver{
		id:     ID,
		name:   name,
		logSrc: observer.NewProperty(nil),
		msgSrc: observer.NewProperty(nil),
	}
}

// WithFactory sets a factory to construct tasks to resolve
func (tr *SimpleResolver) WithFactory(factory TaskFactory) *SimpleResolver {
	if factory != nil {
		tr.task.factory = factory
	}
	return tr
}

// WhenDone set a callback that will be invoked before result is set
func (tr *SimpleResolver) WhenDone(cb func(task Task, result []byte, err error)) *SimpleResolver {
	tr.res.done = cb
	return tr
}

// Start starts a background polling result from input stream
func (tr *SimpleResolver) Start(ctx context.Context) *SimpleResolver {
	return tr.StartWithActionStream(ctx, nil)
}

// StartWithAction feeds a action to output stream and starts polling
func (tr *SimpleResolver) StartWithAction(ctx context.Context, action *messages.Action) *SimpleResolver {
	bts, _ := json.Marshal(action)
	prop := observer.NewProperty(nil)
	stream := prop.Observe()
	prop.Update(bts)

	return tr.StartWithActionStream(ctx, stream)
}

// StartWithActionStream starts feeding actions and polling results
func (tr *SimpleResolver) StartWithActionStream(ctx context.Context, inputActions observer.Stream) *SimpleResolver {
	tr.startOnce.Do(func() {
		tr.ctx, tr.cancel = context.WithCancel(ctx)
		tr.startOnce.t0 = time.Now()
		tr.startOnce.started = true

		if inputActions == nil {
			// create a dummp empty
			inputActions = observer.NewProperty(nil).Observe()
		}

		go tr.ioloop(inputActions)
	})
	return tr
}

// ID implements TaskResolver.ID
func (tr *SimpleResolver) ID() string {
	if tr == nil {
		return ""
	}
	return tr.id
}

// Name implements TaskResolver.Name
func (tr *SimpleResolver) Name() string {
	if tr == nil {
		return ""
	}
	if tr.name != "" {
		return tr.name
	}
	if len(tr.id) > 7 {
		return tr.id[:7]
	}
	return tr.id
}

// Cancel implements TaskResolver.Cancel
func (tr *SimpleResolver) Cancel() {
	if tr != nil {
		tr.SetResult(nil, ErrTaskStopped)
	}
}

// Done implements TaskResolver.Done
func (tr *SimpleResolver) Done() <-chan struct{} {
	if tr == nil || !tr.isStarted() {
		return nil
	}
	return tr.ctx.Done()
}

// LogObserver implements TaskResolver.LogObserver
func (tr *SimpleResolver) LogObserver() observer.Stream {
	return tr.logSrc.Observe()
}

// MsgObserver implements TaskResolver.MsgObserver
func (tr *SimpleResolver) MsgObserver() observer.Stream {
	return tr.msgSrc.Observe()
}

// Result implements TaskResolver.Result
func (tr *SimpleResolver) Result() ([]byte, error) {
	if tr == nil {
		return nil, nil
	}
	return tr.res.data, tr.res.err
}

// StatJSON implements TaskResolver.StatJSON
func (tr *SimpleResolver) StatJSON() string {
	if tr == nil || !tr.isStarted() {
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
		tr.startOnce.t0,
		time.Since(tr.startOnce.t0).String(),
		atomic.LoadUint64(&tr.res.nemit),
		atomic.LoadUint64(&tr.outs.pmsg),
	})
	return string(bts)
}

// SetResult sets result and stopped current task,
// this is one time method once called the task will be shutdown
func (tr *SimpleResolver) SetResult(data []byte, err error) {
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
		if tr.res.done != nil {
			tr.res.done(tr.task.inst, data, err)
		}
		if tr.cancel != nil {
			tr.cancel()
		}
	})
}

// UpdateLog updates log source
func (tr *SimpleResolver) UpdateLog(line []byte) {
	tr.logSrc.Update(unquote(line))
}

func (tr *SimpleResolver) isStarted() bool {
	return tr != nil && tr.startOnce.started
}

func (tr *SimpleResolver) processActions() <-chan struct{} {
	tr.outs.startC = make(chan struct{})
	// poll events from stdout
	go func() {
		close(tr.outs.startC)
		scanner := utils.NewScanner(tr.InputStream)

		for scanner.Scan() {
			if !ParseAction(scanner.Bytes(), tr) {
				return
			}
		}

		if scanner.Err() != nil {
			tr.SetResult(nil, fmt.Errorf("task: scanner error, %v", scanner.Err()))
			return
		}
		tr.SetResult(nil, ErrEmptyResponse)
	}()

	return tr.outs.startC
}

func (tr *SimpleResolver) ioloop(actionSteam observer.Stream) {
	defer func() {
		tr.res.t1 = time.Now()
		msg := fmt.Sprintf("%s done in %v, emit %d msgs", tr.Name(), tr.res.t1.Sub(tr.startOnce.t0), atomic.LoadUint64(&tr.res.nemit))
		plsz := atomic.LoadUint64(&tr.outs.plsz)
		if plsz > 0 {
			msg = fmt.Sprintf("%s, pub %d msg, %s produced", msg, tr.outs.pmsg, utils.ByteSize(plsz))
		}
		if tr.res.err != nil {
			msg = fmt.Sprintf("%s, with error, %v", msg, tr.res.err)
		}
		GetLogger(tr.ctx).Infof(msg)
	}()

	if tr.task.factory != nil {
		task, err := tr.task.factory(tr.ctx)
		if err != nil {
			tr.SetResult(nil, err)
			return
		}
		tr.task.inst = task
		tr.InputStream = task.Stdout()
		tr.OutputStream = task.Stdin()
	}

	if tr.InputStream == nil {
		tr.SetResult(nil, ErrNilInputStream)
		return
	}

	// go and start processing output stream
	<-tr.processActions()

	ctx := tr.ctx
	if _, ok := tr.ctx.Deadline(); !ok {
		timeout := GetTimeoutHint(tr.ctx)
		if timeout > 0 {
			c, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			ctx = c
		}
	}

	for {
		select {
		case <-ctx.Done():
			tr.SetResult(nil, ctx.Err())
			return
		case <-actionSteam.Changes():
		}

		val := actionSteam.Next()
		var bts []byte
		switch msg := val.(type) {
		case nil:
			continue
		case *messages.Action:
			jbts, err := json.Marshal(msg)
			if err != nil {
				tr.SetResult(nil, err)
				return
			}
			bts = jbts
		case []byte:
			bts = msg
		default:
			GetLogger(tr.ctx).Infof("warn unsupported type(%T), %[1]v", val)
			continue
		}

		if tr.OutputStream == nil {
			tr.SetResult(nil, ErrNilOutputStream)
			return
		}

		// write new action, ensure that the action is newline delimited
		_, err := tr.OutputStream.Write(append(bytes.TrimSuffix(bts, messages.TokenCRLF), messages.TokenCRLF...))
		if err != nil {
			tr.SetResult(nil, err)
			return
		}

		flushWriter(tr.OutputStream)
		atomic.AddUint64(&tr.res.nemit, 1)
	}
}
