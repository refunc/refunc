package sidecar

import (
	"context"
	"net/http"

	"k8s.io/klog"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/utils/logtools"
)

// APIVersion for current sidecard
const APIVersion = "2018-06-01"

// Engine is car engine implemented by different transport to commnicate with its operator
type Engine interface {
	// Init engine, that function is loaded
	Init(ctx context.Context, fn *types.Function) error
	// NextC returns a signal channel block and wait for next request
	NextC() <-chan struct{}
	// InvokeRequest consume a request
	InvokeRequest() *messages.InvokeRequest
	// SetResult teminates a reqeust corresponding to its reqeust id (rid)
	SetResult(rid string, body []byte, err error) error
	// ReportInitError runtime encourted a unrecoverable error notify operator to shutdown
	ReportInitError(err error)
	// ReportReady notify operator that we're ready
	ReportReady()
	// ReportExiting notify operator that we're exiting
	ReportExiting()
}

// Loader discovers and loads function runtime config
type Loader interface {
	C() <-chan struct{}
	Function() *types.Function
}

// Sidecar creates a proxy that implements aws lambda runtimes-api
// https://docs.aws.amazon.com/lambda/latest/dg/runtimes-api.html
type Sidecar struct {
	eng    Engine
	loader Loader

	fn *types.Function

	cancel context.CancelFunc
}

// NewCar returns new sidecar from given engine and loader
func NewCar(engine Engine, loader Loader) *Sidecar {
	return &Sidecar{
		eng:    engine,
		loader: loader,
	}
}

// Serve init sidecar waiting for function is ready and listens and serves at given address
func (sc *Sidecar) Serve(ctx context.Context, address string) {
	klog.V(2).Info("(sidecar) start waiting runtime config")
	select {
	case <-ctx.Done():
		klog.V(2).Info("(sidecar) exited with context canceled")
		return
	case <-sc.loader.C():
	}

	fn := sc.loader.Function()
	if fn == nil {
		// unrecoverable
		klog.Fatal("(sidecar) cannot load runtime config")
	}
	sc.fn = fn

	sc.eng.Init(ctx, fn)

	router := mux.NewRouter()

	ctx, sc.cancel = context.WithCancel(ctx)
	sc.reigsterHandlers(router)

	// setup server
	handler := handlers.LoggingHandler(logtools.GlogWriter(2), router)

	// handle proxy
	handler = handlers.ProxyHeaders(handler)

	server := &http.Server{
		Addr:    address,
		Handler: handler,
	}

	go func() {
		klog.Infof("(sidecar) start car at %s", address)
		if err := server.ListenAndServe(); err != nil {
			klog.Errorf("(sidecar) http exited with error, %v", err)
		}
	}()

	sc.eng.ReportReady()

	<-ctx.Done()

	sc.eng.ReportExiting()

	server.Shutdown(ctx)
}
