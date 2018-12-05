package sharedcfg

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/informers"
	k8sinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	clientset "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	rfinformers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

// common vars
var (
	// We'll attempt to recompute EVERY service's endpoints at least this
	// often. Higher numbers = lower CPU/network load; lower numbers =
	// shorter amount of time before a mistaken endpoint is corrected.
	FullyResyncPeriod = time.Minute
)

// Configs provides client resources to use
type Configs interface {
	Context() context.Context
	Namespace() string

	RestConfig() *rest.Config

	KubeClient() kubernetes.Interface
	KubeInformers() k8sinformers.SharedInformerFactory

	RefuncClient() clientset.Interface
	RefuncInformers() rfinformers.SharedInformerFactory
}

// Runner is block and long running object defines a configs consumer
type Runner interface {
	Run(stopC <-chan struct{})
}

// SharedConfigs are collection of k8s configs & clients shared by multiple consumers
type SharedConfigs interface {
	Runner
	Configs() Configs
	AddController(func(cfg Configs) Runner)
}

// New creates a new SharedConfigs
func New(ctx context.Context, ns string) SharedConfigs {
	sr := &sharedRes{
		ctx: ctx,
		ns:  ns,
	}
	// force to get rest config
	sr.RestConfig()
	return sr
}

type sharedRes struct {
	ctx     context.Context
	ns      string
	restCfg *rest.Config

	refuncClient    clientset.Interface
	refuncInformers rfinformers.SharedInformerFactory

	kubeClient    kubernetes.Interface
	kubeInformers k8sinformers.SharedInformerFactory

	controllers []Runner
}

func (sr *sharedRes) Configs() Configs {
	return sr
}

func (sr *sharedRes) Context() context.Context {
	return sr.ctx
}

func (sr *sharedRes) Namespace() string {
	return sr.ns
}

func (sr *sharedRes) RestConfig() *rest.Config {
	if sr.restCfg == nil {
		var err error
		sr.restCfg, err = k8sutil.BuildClusterConfig("", "")
		if err != nil {
			klog.Fatalf("Failed to get restconfig, %v", err)
		}
	}
	return sr.restCfg
}

func (sr *sharedRes) KubeClient() kubernetes.Interface {
	if sr.kubeClient == nil {
		var err error
		sr.kubeClient, err = kubernetes.NewForConfig(sr.RestConfig())
		if err != nil {
			klog.Fatalf("Failed to create kube client, %v", err)
		}
	}
	return sr.kubeClient
}

func (sr *sharedRes) KubeInformers() k8sinformers.SharedInformerFactory {
	if sr.kubeInformers == nil {
		sr.kubeInformers = informers.NewSharedInformerFactoryWithOptions(sr.KubeClient(), FullyResyncPeriod, informers.WithNamespace(sr.Namespace()))
	}
	return sr.kubeInformers
}

func (sr *sharedRes) RefuncClient() clientset.Interface {
	if sr.refuncClient == nil {
		var err error
		sr.refuncClient, err = clientset.NewForConfig(sr.RestConfig())
		if err != nil {
			klog.Fatalf("Failed to create refunc client, %v", err)
		}
	}
	return sr.refuncClient
}

func (sr *sharedRes) RefuncInformers() rfinformers.SharedInformerFactory {
	if sr.refuncInformers == nil {
		sr.refuncInformers = rfinformers.NewSharedInformerFactoryWithOptions(sr.RefuncClient(), FullyResyncPeriod, rfinformers.WithNamespace(sr.Namespace()))
	}
	return sr.refuncInformers
}

func (sr *sharedRes) AddController(fn func(cfg Configs) Runner) {
	sr.controllers = append(sr.controllers, fn(sr))
}

func (sr *sharedRes) Run(stopC <-chan struct{}) {
	var wg sync.WaitGroup
	var ensureControllerStarted sync.WaitGroup
	for _, c := range sr.controllers {
		wg.Add(1)
		ensureControllerStarted.Add(1)
		go func(runner Runner) {
			defer wg.Done()
			ensureControllerStarted.Done()
			runner.Run(stopC)
		}(c)
	}

	ensureControllerStarted.Wait()

	if sr.kubeInformers != nil {
		sr.kubeInformers.Start(stopC)
	}

	if sr.refuncInformers != nil {
		sr.refuncInformers.Start(stopC)
	}

	select {
	case <-stopC:
	case <-sr.ctx.Done():
	}

	wg.Wait()
}

type RunnerFunc func(stopC <-chan struct{})

func (rf RunnerFunc) Run(stopC <-chan struct{}) {
	rf(stopC)
}
