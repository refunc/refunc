package httptrigger

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/allegro/bigcache"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	rfcli "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	informers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	"github.com/refunc/refunc/pkg/messages"
	operators "github.com/refunc/refunc/pkg/operators"
	"github.com/refunc/refunc/pkg/operators/triggers/httptrigger/mmux"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/logtools"
)

// Operator creates http based http trigger automatically for every funcs
type Operator struct {
	*operators.BaseOperator

	// Config cors for http endpoints
	CORS struct {
		AllowedMethods []string
		AllowedHeaders []string
		AllowedOrigins []string
		ExposedHeaders []string

		MaxAge           int
		AllowCredentials bool
	}

	ctx context.Context

	triggers sync.Map

	liveTasks operators.LiveTaskStore

	// http endpoints
	http struct {
		router *mmux.MutableRouter
		cache  httpCache
	}
}

// Type name for http trigger
const Type = "httptrigger"

// NewOperator creates a new http trigger operator
func NewOperator(
	ctx context.Context,
	cfg *rest.Config,
	rclient rfcli.Interface,
	rfInformers informers.SharedInformerFactory,
) (*Operator, error) {
	base, err := operators.NewBaseOperator(cfg, rclient, rfInformers)
	if err != nil {
		return nil, err
	}

	r := &Operator{
		BaseOperator: base,
		ctx:          ctx,
		liveTasks:    operators.NewLiveTaskStore(),
	}

	r.http.router = mmux.NewMutableRouter()
	r.http.cache = new(disabledCache) // lazily creates bigcache in Run()

	return r, nil
}

// Run will not return until stopC is closed.
func (r *Operator) Run(stopC <-chan struct{}) {
	defer func() {
		if re := recover(); re != nil {
			utils.LogTraceback(re, 4, klog.V(1))
		}
	}()

	// create cache
	cfg := bigcache.DefaultConfig(32 * time.Hour)
	cfg.Shards = 32
	cfg.MaxEntriesInWindow = 640
	cfg.MaxEntrySize = int(messages.MaxPayloadSize)
	cfg.HardMaxCacheSize = 10 << 30 // 10G

	bc, err := bigcache.NewBigCache(cfg)
	if err != nil {
		klog.Errorf("(httptrigger) failed to create cache, %v", err)
	} else {
		r.http.cache = bc
	}

	updateHandler := func(fn func(interface{})) func(o, c interface{}) {
		return func(oldObj, curObj interface{}) {
			old, _ := meta.Accessor(oldObj)
			cur, _ := meta.Accessor(curObj)
			if old.GetResourceVersion() == cur.GetResourceVersion() {
				return
			}
			fn(curObj)
		}
	}

	r.RefuncInformers.Refunc().V1beta3().Triggers().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleTriggerAdd,
		UpdateFunc: updateHandler(r.handleTriggerUpdate),
		DeleteFunc: r.handleTriggerDelete,
	})

	klog.Info("(httptrigger) starting http trigger operator")

	if !r.BaseOperator.WaitForCacheSync(stopC) {
		klog.Error("(httptrigger) cannot fully sync resources")
		return
	}

	klog.Info("(httptrigger) starting http server")
	go r.listenAndServe()

	<-stopC
	klog.Info("(httptrigger) shuting down http trigger operator")
}

func (r *Operator) handleTriggerAdd(o interface{}) {
	trigger := o.(*rfv1beta3.Trigger)
	if trigger.Spec.Type != Type {
		// skip other triggers
		return
	}

	key := k8sKey(trigger)
	_, loaded := r.triggers.LoadOrStore(key, &httpHandler{
		fndKey:   trigger.Namespace + "/" + trigger.Spec.FuncName,
		ns:       trigger.Namespace,
		name:     trigger.Name,
		operator: r,
	})
	if !loaded {
		klog.V(3).Infof("(httptrigger) add new trigger %s", key)
		r.popluateEndpoints()
	}
}

func (r *Operator) handleTriggerUpdate(o interface{}) {
	trigger := o.(*rfv1beta3.Trigger)
	if trigger.Spec.Type != Type {
		// skip other triggers
		return
	}
}

func (r *Operator) handleTriggerDelete(o interface{}) {
	trigger, ok := o.(*rfv1beta3.Trigger)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}
	if trigger.Spec.Type != Type {
		// skip other triggers
		return
	}

	key := k8sKey(trigger)
	if _, ok := r.triggers.Load(key); ok {
		klog.V(3).Infof("(httptrigger) delete trigger %s", key)
		r.triggers.Delete(key)
		r.popluateEndpoints()
	}
}

func (r *Operator) popluateEndpoints() {
	router := mux.NewRouter()
	r.triggers.Range(func(_, value interface{}) bool {
		value.(*httpHandler).setupHTTPEndpoints(router)
		return true
	})
	// swap
	r.http.router.UpdateRouter(router)
}

func k8sKey(o metav1.Object) string {
	return o.GetNamespace() + "/" + o.GetName()
}

func fndKey(trigger *rfv1beta3.Trigger) string {
	return trigger.Namespace + "/" + trigger.Spec.FuncName
}

// DefaultRequestReadTimeout for http server
const DefaultRequestReadTimeout = 9 * time.Second

func (r *Operator) listenAndServe() {
	const port = 7788
	url := fmt.Sprintf(":%v", port)
	klog.Infof("(httptrigger) hosting api at: %q", url)

	// setup handlers
	var handler http.Handler = r.http.router

	// config cors
	var corsOpts []handlers.CORSOption
	if r.CORS.AllowCredentials {
		corsOpts = append(corsOpts, handlers.AllowCredentials())
	}
	if r.CORS.MaxAge > 0 {
		corsOpts = append(corsOpts, handlers.MaxAge(r.CORS.MaxAge))
	}
	if len(r.CORS.AllowedOrigins) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedOrigins(r.CORS.AllowedOrigins))
	}
	if len(r.CORS.AllowedMethods) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedMethods(r.CORS.AllowedMethods))
	}
	if len(r.CORS.AllowedHeaders) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedHeaders(r.CORS.AllowedHeaders))
	}
	if len(r.CORS.ExposedHeaders) > 0 {
		corsOpts = append(corsOpts, handlers.ExposedHeaders(r.CORS.ExposedHeaders))
	}
	if len(corsOpts) > 0 {
		klog.Infof("(httptrigger) CORS enabled, %v", r.CORS)
		handler = handlers.CORS(corsOpts...)(handler)
	}

	// logging
	handler = handlers.LoggingHandler(logtools.GlogWriter(2), handler)

	// handle proxy
	handler = handlers.ProxyHeaders(handler)

	server := &http.Server{
		Addr:    url,
		Handler: handler,
	}
	if err := server.ListenAndServe(); err != nil {
		klog.Errorf("(httptrigger) http exited with error, %v", err)
	}
}
