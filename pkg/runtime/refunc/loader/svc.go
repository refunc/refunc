package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k8s.io/klog"

	nats "github.com/nats-io/go-nats"
	observer "github.com/refunc/go-observer"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

var emptyProp = observer.NewProperty(nil)

func (ld *Agent) serve(fnrt *FuncRuntime, opener func() (*exec.Cmd, error), conn *nats.Conn) {
	var (
		cryEndpoint    = os.Getenv("REFUNC_CRY_ENDPOINT")
		tapEndpoint    = os.Getenv("REFUNC_TAP_ENDPOINT")
		logEndpoint    = os.Getenv("REFUNC_LOG_ENDPOINT")
		svcEndpoint    = os.Getenv("REFUNC_SVC_ENDPOINT")
		crysvcEndpoint = os.Getenv("REFUNC_CRY_SVC_ENDPOINT")
	)

	klog.Infof("(agent) cry endpoint: %s", cryEndpoint)
	klog.Infof("(agent) svc endpoint: %s", svcEndpoint)

	// random pick up a valid server override REFUNC_NATS_ENDPOINT to accelerate func startup
	servers := conn.Servers()
	natsURL, _ := url.Parse(servers[rand.Intn(len(servers))])
	os.Setenv("REFUNC_NATS_ENDPOINT", natsURL.Host)

	// helper func
	replyError := func(reply string, err error) {
		klog.V(3).Infof("(agent) request on error, %v", err)
		if err := conn.Publish(reply, messages.GetErrActionBytes(err)); err != nil {
			klog.Errorf("(agent) failed to reply error to %q, %v", reply, err)
			return
		}
	}

	logFn := func(line string) {
		// filter out verbose
		if strings.Contains(line, "] done in") {
			return
		}
		if err := conn.Publish(logEndpoint, []byte(line)); err != nil {
			klog.Errorf("(agent) logging failed %v: %q", err, line)
			return
		}
	}
	logger := &fwdLogger{
		logfn: logFn,
	}

	var (
		timeout = messages.DefaultJobTimeout
		tapProp = observer.NewProperty(struct{}{})
	)
	tap := func() {
		tapProp.Update(struct{}{})
	}
	if fnrt.Spec.Runtime.Timeout > 0 {
		timeout = time.Second * time.Duration(fnrt.Spec.Runtime.Timeout)
	}

	// simple process pool
	var (
		poolMux     sync.Mutex
		placeHolder = struct{}{}
	)
	procPool := make(map[*process]struct{})
	getProc := func() *process {
		poolMux.Lock()
		if len(procPool) == 0 {
			poolMux.Unlock()
			return nil
		}
		for p := range procPool {
			delete(procPool, p)
			poolMux.Unlock()
			return p
		}
		poolMux.Unlock()
		return nil
	}
	putPool := func(p *process) {
		poolMux.Lock()
		if _, ok := procPool[p]; !ok {
			procPool[p] = placeHolder
			klog.V(3).Infof("recycled process %d", p.PID())
		}
		poolMux.Unlock()
	}

	// setup handler to respond request
	sub, err := conn.QueueSubscribe(svcEndpoint, "_svc_", func(msg *nats.Msg) {
		if msg.Reply == "" {
			klog.Errorf("(agent) got invalid request, empty reply")
			return
		}
		reply := msg.Reply

		// verify request
		var req *messages.InvokeRequest
		err := json.Unmarshal(msg.Data, &req)
		if err != nil {
			replyError(reply, err)
			return
		}

		// go and process
		go func() {
			tap()
			proc := getProc()
			if proc == nil {
				p, err := opener()
				if err != nil {
					replyError(reply, err)
					return
				}
				proc = &process{
					proc:  p,
					logfn: logFn,
				}
				// start process
				if err = proc.Start(); err != nil {
					replyError(reply, err)
					return
				}
			}

			var logstream observer.Stream
			logFwd, ok := req.Options["logEndpoint"].(string)
			if !ok || logFwd == "" {
				logstream = emptyProp.Observe()
			} else {
				logstream = proc.logProp.Observe()
			}

			// create and start a task resolver
			tr := client.NewSimpleResolver(
				req.RequestID,
				fmt.Sprintf("%s[%d]", fnrt.Name, proc.PID()),
			).WithFactory(
				func(ctx context.Context) (client.Task, error) { return proc, nil },
			).WhenDone(func(task client.Task, _ []byte, err error) {
				if task == nil {
					return
				}
				p := task.(*process)
				if err != nil {
					p.Close()
					return
				}
				putPool(p)
			}).StartWithAction(
				client.WithTimeoutHint(client.WithLogger(ld.ctx, logger), timeout),
				&messages.Action{
					Type:    messages.Request,
					Payload: messages.MustFromObject(req),
				},
			)

			var (
				publish = func(endpoint string, bts []byte) {
					if err := conn.Publish(endpoint, bts); err != nil {
						logger.Errorf("(agent)[%d] publish to %s failed, %v", proc.PID(), endpoint, err)
					}
				}
				ticker = time.NewTicker(time.Second)
			)
			defer ticker.Stop()

			logPrefix := func() string {
				return fmt.Sprintf("%s %s/%s] ", time.Now().UTC().Format("0102T15:04:05.000Z"), fnrt.Namespace, fnrt.Name)
			}

			for {
				select {
				case <-logstream.Changes():
					for logstream.HasNext() {
						publish(logFwd, messages.MustFromObject(&messages.Action{
							Type:    messages.Log,
							Payload: json.RawMessage(logPrefix() + logstream.Next().(string)),
						}))
					}
				case <-tr.Done():
					bts, err := tr.Result()
					if err != nil {
						bts = messages.GetErrActionBytes(err)
					} else {
						tap()
					}
					publish(reply, bts)
					return

				case <-ticker.C:
					tap()
				}
			}
		}()
	})
	if err != nil {
		klog.Fatalf("(agent) failed to subscribe on %s, %v", svcEndpoint, err)
	}
	defer sub.Unsubscribe()

	// setup handler to respond cry request
	crySubs, err := conn.QueueSubscribe(crysvcEndpoint, "_svc_", func(msg *nats.Msg) {
		reply := msg.Reply
		if reply == "" {
			reply = cryEndpoint
		}
		conn.Publish(reply, nil)
	})
	if err != nil {
		klog.Fatalf("(agent) failed to subscribe on cry, %v", err)
	}
	defer crySubs.Unsubscribe()

	// explicity send cry message
	err = conn.Publish(cryEndpoint, nil)
	if err != nil {
		klog.Errorf(`(agent) failed to send "cry" message, %v`, err)
	}

	logger.Infof("(agent) %s v2 started", fnrt.Name)
	var (
		tapTicker = time.NewTicker(2 * time.Second)
		tapStream = tapProp.Observe()
	)
	defer tapTicker.Stop()
	for {
		select {
		case <-ld.ctx.Done():
			// wait until we are requested to leave
			return
		case <-tapTicker.C:
			var tapOnce sync.Once
			for tapStream.HasNext() {
				tapStream.Next()
				tapOnce.Do(func() {
					if err := conn.Publish(tapEndpoint, nil); err != nil {
						logger.Errorf("(agent) %s tap failed, %v", fnrt.Name, err)
					}
				})
			}
		}
	}
}

type process struct {
	proc *exec.Cmd

	stdin  io.WriteCloser
	stdout io.Reader
	stderr io.Reader

	logfn func(string)
	// logging endpoint attached to stderr
	logProp observer.Property
}

var _ client.Task = (*process)(nil)

func (p *process) Start() (err error) {

	p.stdout, err = p.proc.StdoutPipe()
	if err != nil {
		return
	}

	p.stderr, err = p.proc.StderrPipe()
	if err != nil {
		return
	}

	p.stdin, err = p.proc.StdinPipe()
	if err != nil {
		return
	}

	err = p.proc.Start()
	if err != nil {
		return
	}

	// start log scanner
	p.logProp = observer.NewProperty(messages.TokenCRLF)

	// logging from stderr
	go func() {
		scanner := utils.NewScanner(p.stderr)
		for scanner.Scan() {
			line := scanner.Text()
			p.logfn(line)
			p.logProp.Update(line)
		}
	}()

	return nil
}

func (p *process) Stdin() io.WriteCloser {
	return p.stdin
}

func (p *process) Stdout() io.Reader {
	return p.stdout
}

func (p *process) PID() int {
	if p.proc.Process != nil {
		return p.proc.Process.Pid
	}
	return -1
}

func (p *process) Close() error {
	if p == nil || p.proc.Process == nil {
		return nil
	}
	klog.V(3).Infof("(w)[%d] closed", p.PID())
	p.stdin.Close()

	if err := p.proc.Process.Kill(); err != nil {
		return err
	}
	// wait until process closed
	return p.proc.Wait()
}

type fwdLogger struct {
	logfn func(string)
}

func (g fwdLogger) Error(args ...interface{}) {
	klog.Error(args...)
	g.logfn(fmt.Sprint(args...))
}

func (g fwdLogger) Errorf(format string, args ...interface{}) {
	klog.Errorf(format, args...)
	g.logfn(fmt.Sprintf(format, args...))
}

func (g fwdLogger) Info(args ...interface{}) {
	klog.Info(args...)
	g.logfn(fmt.Sprint(args...))
}

func (g fwdLogger) Infof(format string, args ...interface{}) {
	klog.Infof(format, args...)
	g.logfn(fmt.Sprintf(format, args...))
}
