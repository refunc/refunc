package loader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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
	cmd, err := ld.prepare(fn)
	if err != nil {
		return err
	}

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

			body, err := ioutil.ReadAll(res.Body)
			if err != nil || string(body) != "pong" {
				return errors.New("loader: failed to reqeust api")
			}
			break
		}
	}

	klog.Infof("(loader) exec %s", strings.Join(cmd.Args, " "))
	return cmd.Run()
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
		w.Write(messages.MustFromObject(errMsg))
		w.(http.Flusher).Flush()
		// shutdown do not try anymore
		if code >= 500 {
			select {
			case errC <- err:
			default: // void closed panic
			}
		}
	}

	router.Path("/init").Methods(http.MethodPost).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var fn types.Function
		if err := json.NewDecoder(r.Body).Decode(&fn); err != nil {
			writeError(w, http.StatusBadRequest, err, "")
			return
		}

		// capture logs
		buf, err := func() ([]byte, error) {
			buf := bytes.NewBuffer(nil)
			klogWriter.Switch(buf)
			err := ld.setup(&fn)
			klog.Flush()
			klogWriter.Switch(nil)
			return buf.Bytes(), err
		}()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err, fmt.Sprintf("loader: %v\r\n%s", err, string(buf)))
			w.(http.Flusher).Flush()
			return
		}

		select {
		case fnC <- &fn:
			w.WriteHeader(http.StatusOK)
			w.Write(buf)
			w.Write(messages.TokenCRLF)
		default: // avoid panic
			w.WriteHeader(http.StatusConflict)
			w.Write(messages.MustFromObject(messages.GetErrorMessage(errors.New("loader: already initialized"))))
		}

		w.(http.Flusher).Flush()
	})

	// setup server
	handler := handlers.LoggingHandler(logtools.GlogWriter(2), router)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	defer func() {
		go server.Shutdown(context.Background())
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
