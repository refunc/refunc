package controller

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/controllers/funcinst"
	"github.com/refunc/refunc/pkg/controllers/xenv"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
	"github.com/refunc/refunc/pkg/version"
	"github.com/spf13/cobra"
)

// env keys
const (
	EnvMyPodName      = "REFUNC_NAME"
	EnvMyPodNamespace = "REFUNC_NAMESPACE"

	// We should start gc at given interval to free unused resources
	DefaultGCPeriod = 2 * time.Minute

	// DefaultIdleDuraion is default value of lifetime for a refunc
	DefaultIdleDuraion = 3 * DefaultGCPeriod
)

// NewCmd creates new commands
func NewCmd() *cobra.Command {
	var config struct {
		GCInterval  time.Duration
		IdleDuraion time.Duration
		Workers     int
		Namespace   string
	}

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "the refunc controller",
		Long:  `the refunc controller`,
		Run: func(cmd *cobra.Command, args []string) {
			namespace := os.Getenv(EnvMyPodNamespace)
			if len(namespace) == 0 {
				klog.Fatalf("Must set env (%s)", EnvMyPodNamespace)
			}
			name := os.Getenv(EnvMyPodName)
			if len(name) == 0 {
				klog.Fatalf("Must set env (%s)", EnvMyPodName)
			}
			id, err := os.Hostname()
			if err != nil {
				klog.Fatalf("Failed to get hostname: %v", err)
			}

			if config.Workers <= 0 {
				config.Workers = 1
			}

			if config.IdleDuraion <= 0 {
				config.IdleDuraion = DefaultIdleDuraion
			}

			ctx, cancel := context.WithCancel(context.Background())

			sc := sharedcfg.New(ctx, config.Namespace)

			ensureNSCreated(sc.Configs().RestConfig())
			ensureCRDsCreated(sc.Configs().RestConfig())

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				// create funcinst controller
				fnic, err := funcinst.NewController(
					cfg.RestConfig(),
					cfg.RefuncClient(),
					cfg.KubeClient(),
					cfg.RefuncInformers(),
					cfg.KubeInformers(),
				)
				if err != nil {
					klog.Fatalf("Failed to create funcinst controller, %v", err)
				}
				fnic.GCInterval = config.GCInterval
				fnic.IdleDuraion = config.IdleDuraion
				return sharedcfg.RunnerFunc(func(stopC <-chan struct{}) {
					fnic.Run(config.Workers, stopC)
				})
			})

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				// create xenv controller
				xnc, err := xenv.NewController(
					cfg.RestConfig(),
					cfg.RefuncClient(),
					cfg.KubeClient(),
					cfg.RefuncInformers(),
					cfg.KubeInformers(),
				)
				if err != nil {
					klog.Fatalf("Failed to create funcinst controller, %v", err)
				}
				xnc.GCInterval = config.GCInterval
				xnc.IdleDuraion = config.IdleDuraion
				return sharedcfg.RunnerFunc(func(stopC <-chan struct{}) {
					xnc.Run(config.Workers, stopC)
				})
			})

			var wg sync.WaitGroup
			run := func(ctx context.Context) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					klog.Infof("Refunc  Version: %s", version.Version)
					klog.Infof("Loader  Version: %s", version.LoaderVersion)
					klog.Infof("Sidecar Version: %s", version.SidecarVersion)
					sc.Run(ctx.Done())
				}()

				klog.Infof(`Received signal "%v", exiting...`, <-cmdutil.GetSysSig())
				cancel()
				os.Exit(0)
			}

			rl, err := resourcelock.New(
				resourcelock.EndpointsResourceLock,
				namespace,
				"refunc-controllers",
				sc.Configs().KubeClient().CoreV1(),
				sc.Configs().KubeClient().CoordinationV1(),
				resourcelock.ResourceLockConfig{
					Identity: id,
					EventRecorder: k8sutil.CreateRecorder(
						sc.Configs().KubeClient(),
						name,
						namespace,
					),
				},
			)
			if err != nil {
				klog.Fatalf("Fail to create lock, %v", err)
			}

			leaderelection.RunOrDie(ctx,
				leaderelection.LeaderElectionConfig{
					Lock:          rl,
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
					Callbacks: leaderelection.LeaderCallbacks{
						OnStartedLeading: run,
						OnStoppedLeading: func() {
							klog.Info("Stop leading, exit")
							cancel()
							wg.Wait()
						},
						OnNewLeader: func(identity string) {
							klog.Infof("New leader %q detected", identity)
						},
					},
				},
			)
		},
	}
	cmd.Flags().DurationVar(&config.GCInterval, "gc-interval", DefaultGCPeriod, "The interval bewteen each gc")
	cmd.Flags().DurationVar(&config.IdleDuraion, "idle-duration", DefaultIdleDuraion, "The lifetime for a active refunc")
	cmd.Flags().IntVar(&config.Workers, "workers", runtime.NumCPU(), "The number of workers")
	cmd.Flags().StringVarP(&config.Namespace, "namespace", "n", "", "The scope of namepsace to manipulate")
	return cmd
}

func ensureNSCreated(cfg *rest.Config) {
	// ensure and wait tprs
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to ensuring namespaces created, %v", err)
	}

	const ns = "refunc"

	var namespace v1.Namespace
	namespace.SetName(ns)

	if _, err := clientset.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		klog.Fatalf("Failed to ensuring namespaces created, %v", err)
	}
}

func ensureCRDsCreated(cfg *rest.Config) {
	clientset, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to ensuring CRDs created, %v", err)
	}

	defer func() {
		if err != nil {
			klog.Fatalf("Failed to ensuring CRDs created, %v", err)
			return
		}
	}()

	crdcli := clientset.ApiextensionsV1beta1().CustomResourceDefinitions()
	for _, crd := range rfv1beta3.CRDs {
		if _, err = crdcli.Create(context.TODO(), crd.CRD, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return
		}
		// wait for ready
		if err = waitCRDEstablished(clientset, crd.CRD.GetName()); err != nil {
			return
		}
		klog.Infof("CRD %s created", crd.Name)
	}
	return
}

func waitCRDEstablished(clientset *apiextensionsclient.Clientset, name string) error {
	// wait for CRD being established
	return wait.Poll(100*time.Millisecond, 60*time.Second, func() (bool, error) {
		crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, err
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					return false, fmt.Errorf("Name conflict: %v", cond.Reason)
				}
			}
		}
		return false, err
	})
}
