package sidecar

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"sync"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/utils/logtools"
	"k8s.io/klog"
)

// APIVersion for current sidecard
const APIVersion = "2018-06-01"
const RefuncRoot = "/var/run/refunc"

// Engine is car engine implemented by different transport to commnicate with its operator
type Engine interface {
	// Name of current engine
	Name() string

	// Init engine, that function is loaded
	Init(ctx context.Context, fn *types.Function) error
	// RegisterServices exports engine's extended services, will mount at <prefix>/<Engine.Name()>
	RegisterServices(router *mux.Router)
	// ReportInitError runtime encourted a unrecoverable error notify operator to shutdown
	ReportInitError(err error)
	// ReportReady notify operator that we're ready
	ReportReady()
	// ReportExiting notify operator that we're exiting
	ReportExiting()

	// NextC returns a signal channel block and wait for next request
	NextC() <-chan struct{}
	// InvokeRequest consume a request
	InvokeRequest() *messages.InvokeRequest
	// SetResult teminates a reqeust corresponding to its reqeust id (rid)
	SetResult(rid string, body []byte, err error, conentType string) error
	// WriteLog collect func's worker process (wid) log
	WriteLog(wid string, bts []byte)
	// ForwardLog collect func's log for request
	ForwardLog(endpoint string, bts []byte)
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

	logForwards sync.Map
	logStreams  sync.Map

	cancel context.CancelFunc
}

// NewCar returns new sidecar from given engine and loader
func NewCar(engine Engine, loader Loader) *Sidecar {
	return &Sidecar{
		eng:    engine,
		loader: loader,
	}
}

type serverFactor func() (serve func(http.Handler) error, shutdown func(context.Context) error)

// Serve init sidecar waiting for function is ready and listens and serves at given address
func (sc *Sidecar) Serve(ctx context.Context, address string) {
	sc.start(ctx, func() (func(http.Handler) error, func(context.Context) error) {
		server := &http.Server{}
		return func(handler http.Handler) error {
			klog.Infof("(sidecar) start car at %s", address)
			server.Addr = address
			server.Handler = handler
			return server.ListenAndServe()
		}, server.Shutdown
	})
}

// ServeListener init sidecar waiting for function is ready and listens and serves at given listener
func (sc *Sidecar) ServeListener(ctx context.Context, listener net.Listener) {
	sc.start(ctx, func() (func(http.Handler) error, func(context.Context) error) {
		server := &http.Server{}
		return func(handler http.Handler) error {
			klog.Info("(sidecar) start car using provided listener")
			server.Handler = handler
			return server.Serve(listener)
		}, server.Shutdown
	})
}

func (sc *Sidecar) start(ctx context.Context, factory serverFactor) {
	klog.V(2).Info("(sidecar) start waiting runtime config")
	select {
	case <-ctx.Done():
		klog.V(2).Info("(sidecar) exited with context canceled")
		return
	case <-sc.loader.C():
	}

	fn := sc.loader.Function()
	if fn == nil {
		sc.eng.ReportInitError(fmt.Errorf("sidecar: cannot load runtime config"))
		return
	}
	sc.fn = fn

	router := mux.NewRouter()
	sc.reigsterHandlers(router)

	ctx, sc.cancel = context.WithCancel(ctx)

	if err := sc.eng.Init(ctx, fn); err != nil {
		// unrecoverable
		klog.Fatalf("(sidecar) cannot init engine, %v", err)
	}
	sc.eng.RegisterServices(router.PathPrefix(path.Join("/", sc.eng.Name())).Subrouter())

	// setup server
	handler := handlers.LoggingHandler(logtools.GlogWriter(logtools.GoLogLevel), router)

	// handle proxy
	handler = handlers.ProxyHeaders(handler)

	serve, shutdown := factory()

	go func() {
		defer sc.cancel()

		if err := serve(handler); err != nil {
			klog.Errorf("(sidecar) http exited with error, %v", err)
		}
	}()

	sc.eng.ReportReady()

	<-ctx.Done()

	sc.eng.ReportExiting()

	shutdown(ctx) //nolint:errcheck
}
