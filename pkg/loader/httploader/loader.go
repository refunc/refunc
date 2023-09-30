package httploader

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/loader"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/utils/logtools"
	"k8s.io/klog"
)

const (
	// ConfigFile if file name of config to load
	ConfigFile = "refunc.json"
	// DefaultPath default folder to search config file
	DefaultPath = "/var/run/refunc"
)

type httpLoader struct {
	c    chan struct{}
	fn   *types.Function
	path string
}

func (l *httpLoader) C() <-chan struct{} {
	return l.c
}

func (l *httpLoader) Function() *types.Function {
	return l.fn
}

func (l *httpLoader) setup() {
	var fn types.Function
	fn.ObjectMeta = l.fn.ObjectMeta
	fn.Spec.Body, fn.Spec.Hash, fn.Spec.Cmd = l.fn.Spec.Body, l.fn.Spec.Hash, l.fn.Spec.Cmd
	fn.Spec.Runtime.Envs = map[string]string{
		"_HANDLER":                    l.fn.Spec.Runtime.Envs["_HANDLER"],
		"AWS_LAMBDA_FUNCTION_HANDLER": l.fn.Spec.Runtime.Envs["_HANDLER"],
		"AWS_LAMBDA_RUNTIME_API":      l.fn.Spec.Runtime.Envs["AWS_LAMBDA_RUNTIME_API"],
	}
	for key, value := range l.fn.Spec.Runtime.Envs {
		if strings.HasPrefix(key, "REFUNC_") || strings.HasPrefix(key, "AWS_") {
			continue
		}
		fn.Spec.Runtime.Envs[key] = value
	}
	bts, err := json.Marshal(fn)
	if err != nil {
		klog.Errorf("(httploader) setup funcdef marshal error, %v", err)
		return
	}
	if err := os.WriteFile(l.path, bts, 0644); err != nil {
		klog.Errorf("(httploader) setup funcdef write error, %v", err)
	}
	klog.Infof("(httploader) setup funcdef to %s", l.path)
}

func NewLoader(ctx context.Context, addr string, folder string) (loader.Loader, error) {
	if folder == "" {
		folder = DefaultPath
	}
	l := new(httpLoader)
	l.c = make(chan struct{})
	l.path = filepath.Join(folder, ConfigFile)

	router := mux.NewRouter()
	writeError := func(w http.ResponseWriter, code int, err error, msg string) {
		w.WriteHeader(code)
		errMsg := messages.GetErrorMessage(err)
		if msg != "" {
			// override message
			errMsg.Message = msg
		}
		w.Write(messages.MustFromObject(errMsg)) //nolint:errcheck
		w.(http.Flusher).Flush()
	}

	var initOnce sync.Once
	router.Path("/init").Methods(http.MethodPost).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initOnce.Do(func() {
			defer close(l.c)
			var fn types.Function
			if err := json.NewDecoder(r.Body).Decode(&fn); err != nil {
				writeError(w, http.StatusBadRequest, err, "")
				return
			}

			l.fn = &fn

			buf := bytes.NewBuffer(nil)
			klogWriter.Switch(buf)
			klog.Flush()
			klogWriter.Switch(nil)

			w.WriteHeader(http.StatusOK)
			w.Write(buf.Bytes())        //nolint:errcheck
			w.Write(messages.TokenCRLF) //nolint:errcheck

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

	go func() {
		<-l.c
		if l.fn == nil {
			klog.Error("(httploader) setup funcdef error, fn is nil")
			return
		}
		l.setup()
	}()

	// kickoff and serve
	go func() {
		klog.Infof("(httploader) starting server at %s, waiting for requests", server.Addr)
		if err := server.ListenAndServe(); checkServerErr(err) != nil {
			klog.Errorf("(httploader) http exited with error, %v", err)
			return
		}
		klog.Info("(httploader) server exited")
	}()
	return l, nil
}
