package xenv

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	refunc "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	rfinformers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	rflistersv1 "github.com/refunc/refunc/pkg/generated/listers/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils"
)

// Controller manages xenvs.
type Controller struct {
	GCInterval  time.Duration
	IdleDuraion time.Duration

	cfg rest.Config // keep a copy of config

	rclient refunc.Interface
	kclient kubernetes.Interface

	// stores
	kubeInformers   k8sinformers.SharedInformerFactory
	refuncInformers rfinformers.SharedInformerFactory

	deploymentLister v1beta1.DeploymentLister

	funcdefLister rflistersv1.FuncdefLister
	xenvLister    rflistersv1.XenvLister

	// working queue, synced tasks
	queue           workqueue.RateLimitingInterface
	wantedInformers []cache.InformerSynced
}

// NewController creates a new refunc controller from config
func NewController(
	cfg *rest.Config,
	rclient refunc.Interface,
	kclient kubernetes.Interface,
	refuncInformers rfinformers.SharedInformerFactory,
	kubeinformers k8sinformers.SharedInformerFactory,
) (rc *Controller, err error) {
	r := &Controller{
		cfg:   *cfg,
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "xenvs"),
	}
	r.kclient = kclient
	r.rclient = rclient
	r.kubeInformers = kubeinformers
	r.refuncInformers = refuncInformers

	// config listers
	r.deploymentLister = kubeinformers.Extensions().V1beta1().Deployments().Lister()

	r.funcdefLister = refuncInformers.Refunc().V1beta3().Funcdeves().Lister()
	r.xenvLister = refuncInformers.Refunc().V1beta3().Xenvs().Lister()

	// config handlers
	updateHandler := func(fn func(interface{})) func(o, c interface{}) {
		return func(oldObj, curObj interface{}) {
			old, _ := meta.Accessor(oldObj)
			cur, _ := meta.Accessor(curObj)

			// Periodic resync may resend the deployment without changes in-between.
			// Also breaks loops created by updating the resource ourselves.
			if old.GetResourceVersion() == cur.GetResourceVersion() {
				return
			}
			fn(curObj)
		}
	}

	kubeinformers.Extensions().V1beta1().Deployments().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleDeploymentChange,
		DeleteFunc: r.handleDeploymentChange,
	})

	refuncInformers.Refunc().V1beta3().Funcdeves().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleFuncdefChange,
		UpdateFunc: r.handleFuncdefUpdate,
		DeleteFunc: r.handleFuncdefChange,
	})

	refuncInformers.Refunc().V1beta3().Xenvs().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleChange,
		UpdateFunc: updateHandler(r.handleChange),
		DeleteFunc: r.handleChange,
	})

	r.wantedInformers = []cache.InformerSynced{
		r.refuncInformers.Refunc().V1beta3().Funcdeves().Informer().HasSynced,
		r.refuncInformers.Refunc().V1beta3().Xenvs().Informer().HasSynced,
		r.kubeInformers.Extensions().V1beta1().Deployments().Informer().HasSynced,
	}

	return r, nil
}

// Run will not return until stopC is closed.
func (rc *Controller) Run(workers int, stopC <-chan struct{}) {
	klog.Info("(xc) starting xenv controller")
	defer klog.Info("(xc) shuting down xenv controller")

	defer utils.HandleCrash()
	defer rc.queue.ShutDown()

	klog.Info("(xc) waiting for stores to be fully synced")
	if !cache.WaitForCacheSync(stopC, rc.wantedInformers...) {
		return
	}

	// collect orphans
	rc.collectGarbadge()

	klog.Infof("(xc) starting #%d workers", workers)
	for i := 0; i < workers; i++ {
		go wait.Until(rc.worker, time.Second, stopC)
	}

	go rc.gcMonitor(stopC)

	<-stopC
}

func (rc *Controller) worker() {
	for rc.processNextItem() {
	}
}

func (rc *Controller) processNextItem() bool {
	key, quit := rc.queue.Get()
	if quit {
		return false
	}
	defer rc.queue.Done(key)

	err := rc.sync(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	rc.handleErr(key, err)
	return true
}

func (rc *Controller) handleErr(key interface{}, err error) {
	const (
		// Copy from deployment_controller.go:
		// maxRetries is the number of times a restore request will be retried before it is dropped out of the queue.
		// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
		// an restore request is going to be requeued:
		//
		// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
		maxRetries = 15
	)

	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		rc.queue.Forget(key)
		return
	}

	// This controller retries maxRetries times if something goes wrong. After that, it stops trying.
	if rc.queue.NumRequeues(key) < maxRetries {
		klog.Errorf("error syncing funcinst request (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		rc.queue.AddRateLimited(key)
		return
	}

	rc.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	klog.Infof("(xc) dropping funcinst request (%v) out of the queue: %v", key, err)
}

func (rc *Controller) keyFunc(obj interface{}) (string, bool) {
	k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		klog.Errorf("(xc) creating key failed: %v", err)
		return k, false
	}
	return k, true
}

// enqueue adds a key to the queue. If obj is a key already it gets added directly.
// Otherwise, the key is extracted via keyFunc.
func (rc *Controller) enqueue(obj interface{}, reason string) {
	if obj == nil {
		return
	}

	key, ok := obj.(string)
	if !ok {
		key, ok = rc.keyFunc(obj)
		if !ok {
			return
		}
	}

	klog.V(4).Infof("(tc) %q enqueued for <%s>", key, reason)
	rc.queue.Add(key)
}
