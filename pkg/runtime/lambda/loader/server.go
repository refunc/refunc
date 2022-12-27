package loader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k8s.io/klog"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/utils/logtools"
)

// Loader listens to address to load res and handles the bootstrap of a func
type Loader interface {
	Start(ctx context.Context, addr string) error
}

// NewSimpleLoader creates a new simple loader,
// simple loader will listen POST reqeust at /init, and download resouces
func NewSimpleLoader(main, taskRoot, runtimeRoot, layersRoot string) Loader {
	return &simpleLoader{
		Main:        main,
		TaskRoot:    taskRoot,
		RuntimeRoot: runtimeRoot,
		LayersRoot:  layersRoot,
	}
}

type simpleLoader struct {
	Main string

	TaskRoot    string
	RuntimeRoot string
	LayersRoot  string

	ctx context.Context
}

// common vars
var (
	RefuncRoot = "/var/run/refunc"
	ConfigFile = "refunc.json"

	DefaultMain        = "/var/runtime/bootstrap"
	AlterMainPath      = "/opt/bootstrap"
	DefaultTaskRoot    = "/var/task"
	DefaultRuntimeRoot = "/var/runtime"
	DefaultLayersRoot  = "/opt"
)

func (ld *simpleLoader) Start(ctx context.Context, addr string) error {
	ld.ctx = ctx
	fn, err := ld.loadFunc()
	if err != nil {
		klog.Infof("(loader) cannot load function, %v", err)
		if fn, err = ld.wait(addr); err != nil {
			return err
		}
	}
	return ld.exec(fn)
}

func (ld *simpleLoader) exec(fn *types.Function) error {
	if apiAddr := fn.Spec.Runtime.Envs["AWS_LAMBDA_RUNTIME_API"]; apiAddr != "" {
		klog.Infoln("(loader) ping api")
		for i := 0; i < 200; i++ {
			res, err := http.Get("http://" + apiAddr + "/2018-06-01/ping")
			if err != nil {
				select {
				case <-ld.ctx.Done():
					return ld.ctx.Err()
				case <-time.After(5 * time.Millisecond):
					continue
				}
			}
			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil || string(body) != "pong" {
				return errors.New("loader: failed to reqeust api")
			}
			break
		}
	}

	concurrency := withConcurrency(fn)
	cmds := []*exec.Cmd{}
	for i := 0; i < concurrency; i++ {
		cmd, err := ld.prepare(fn)
		if err != nil {
			return err
		}
		cmds = append(cmds, cmd)
	}

	wg := sync.WaitGroup{}
	for i, cmd := range cmds {
		wg.Add(1)
		go func(wid int, c *exec.Cmd) {
			klog.Infof("(loader) worker #%d exec %s", wid, strings.Join(c.Args, " "))
			if err := c.Run(); err != nil {
				klog.Errorf("(loader) worker #%d exec %s error %v", wid, strings.Join(c.Args, " "), err)
			}
			wg.Done()
		}(i, cmd)
	}
	wg.Wait()

	return errors.New("(loader) all workers exit")
}

func (ld *simpleLoader) wait(addr string) (*types.Function, error) {
	router := mux.NewRouter()

	fnC := make(chan *types.Function)
	defer close(fnC)
	errC := make(chan error)
	defer close(errC)

	writeError := func(w http.ResponseWriter, code int, err error, msg string) {
		w.WriteHeader(code)
		errMsg := messages.GetErrorMessage(err)
		if msg != "" {
			// override message
			errMsg.Message = msg
		}
		w.Write(messages.MustFromObject(errMsg)) //nolint:errcheck
		w.(http.Flusher).Flush()
		// shutdown do not try anymore
		if code >= 500 {
			select {
			case errC <- err:
			default: // void closed panic
			}
		}
	}

	var initOnce sync.Once
	router.Path("/init").Methods(http.MethodPost).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initOnce.Do(func() {
			var fn types.Function
			if err := json.NewDecoder(r.Body).Decode(&fn); err != nil {
				writeError(w, http.StatusBadRequest, err, "")
				return
			}

			// capture logs
			buf, err := func() ([]byte, error) {
				buf := bytes.NewBuffer(nil)
				klogWriter.Switch(buf)
				defer klog.Flush()
				defer klogWriter.Switch(nil)
				return buf.Bytes(), ld.setup(&fn)
			}()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err, fmt.Sprintf("loader: %v\r\n%s", err, string(buf)))
				w.(http.Flusher).Flush()
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(buf)                //nolint:errcheck
			w.Write(messages.TokenCRLF) //nolint:errcheck

			fnC <- &fn
			w.(http.Flusher).Flush()
			w = noOpRspWriter{}
		})

		w.WriteHeader(http.StatusOK)
	})

	// setup server
	handler := handlers.LoggingHandler(logtools.GlogWriter(2), router)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	defer func() {
		go server.Shutdown(context.Background()) //nolint:errcheck
	}()

	// kickoff and serve
	go func() {
		klog.Infof("(loader) starting server at %s, waiting for requests", server.Addr)
		if err := server.ListenAndServe(); checkServerErr(err) != nil {
			klog.Errorf("(loader) http exited with error, %v", err)
			return
		}
		klog.Info("(loader) server exited")
	}()

	select {
	case <-ld.ctx.Done():
		return nil, ld.ctx.Err()
	case fn := <-fnC:
		return fn, nil
	case err := <-errC:
		return nil, err
	}
}

func (ld *simpleLoader) mainExe() string {
	if ld.Main != "" {
		return ld.Main
	}
	return DefaultMain
}

func (ld *simpleLoader) taskRoot() string {
	if ld.TaskRoot != "" {
		return ld.TaskRoot
	}
	return DefaultTaskRoot
}

func (ld *simpleLoader) runtimeRoot() string {
	if ld.RuntimeRoot != "" {
		return ld.RuntimeRoot
	}
	return DefaultRuntimeRoot
}

func (ld *simpleLoader) layersRoot() string {
	if ld.LayersRoot != "" {
		return ld.LayersRoot
	}
	return DefaultLayersRoot
}

type noOpRspWriter struct{}

func (noOpRspWriter) Header() http.Header       { return make(http.Header) }
func (noOpRspWriter) Write([]byte) (int, error) { return 0, nil }
func (noOpRspWriter) WriteHeader(int)           {}
