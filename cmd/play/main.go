package play

import (
	"context"
	"os"
	"sync"
	"time"

	"k8s.io/klog"

	nats "github.com/nats-io/nats.go"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/controllers/funcinst"
	"github.com/refunc/refunc/pkg/controllers/xenv"
	"github.com/refunc/refunc/pkg/credsyncer"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/operators/funcinsts"
	"github.com/refunc/refunc/pkg/operators/triggers/crontrigger"
	"github.com/refunc/refunc/pkg/operators/triggers/httptrigger"
	"github.com/refunc/refunc/pkg/transport/natsbased"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv/wrapcobra"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/refunc/refunc/pkg/version"
	"github.com/spf13/cobra"

	// load builtins
	_ "github.com/refunc/refunc/pkg/builtins/helloworld"
	_ "github.com/refunc/refunc/pkg/builtins/sign"
)

// well known default constants
const (
	EnvMyPodName      = "REFUNC_NAME"
	EnvMyPodNamespace = "REFUNC_NAMESPACE"

	// We should start gc at given interval to free unused resources
	DefaultGCPeriod = 2 * time.Minute

	// DefaultIdleDuraion is default value of lifetime for a refunc
	DefaultIdleDuraion = 3 * DefaultGCPeriod
)

var config struct {
	Namespace string
}

// NewCmd creates new commands
func NewCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "play",
		Short: "play refunc in local or dev environment",
		Run: func(cmd *cobra.Command, args []string) {
			// print commands' help
			cmd.Help() // nolint:errcheck
		},
	}
	cmd.AddCommand(wrapcobra.Wrap(genCmd()))
	cmd.AddCommand(wrapcobra.Wrap(startCmd()))

	cmd.PersistentFlags().StringVarP(&config.Namespace, "namespace", "n", "", "The scope of namepsace to manipulate")

	return wrapcobra.Wrap(cmd)
}

func genCmd() *cobra.Command {
	var tplConfig struct {
		Bucket   string
		S3Prefix string
	}
	cmd := &cobra.Command{
		Use:   "gen",
		Short: "generate all-in-one k8s resources for play in local",
		Run: func(cmd *cobra.Command, args []string) {
			if config.Namespace == "" {
				config.Namespace = "refunc-play"
			}
			if err := k8sTpl.Execute(os.Stdout, struct {
				Namespace string
				ImageTag  string
				Bucket    string
				S3Prefix  string
			}{
				Namespace: config.Namespace,
				ImageTag:  version.Version,
				Bucket:    tplConfig.Bucket,
				S3Prefix:  tplConfig.S3Prefix,
			}); err != nil {
				klog.Fatal(err)
			}
		},
	}
	cmd.Flags().StringVar(&tplConfig.Bucket, "bucket", "refunc", "Bucket for minio to store functions")
	cmd.Flags().StringVar(&tplConfig.S3Prefix, "prefix", "functions", "Path prefix(folder) to store functions under bukect")
	return cmd
}

func startCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start all components in one",
		Run: func(cmd *cobra.Command, args []string) {
			namespace := os.Getenv(EnvMyPodNamespace)
			if len(namespace) == 0 {
				klog.Fatalf("Must set env (%s)", EnvMyPodNamespace)
			}
			name := os.Getenv(EnvMyPodName)
			if len(name) == 0 {
				klog.Fatalf("Must set env (%s)", EnvMyPodName)
			}

			natsConn, err := env.NewNatsConn(nats.Name(os.Getenv(EnvMyPodNamespace) + "/" + os.Getenv(EnvMyPodName)))
			if err != nil {
				klog.Fatalf("Failed to connect to nats %s, %v", env.GlobalNatsURLString(), err)
			}
			defer natsConn.Close()

			ctx, cancel := context.WithCancel(context.Background())
			ctx = client.WithNatsConn(ctx, natsConn)

			sc := sharedcfg.New(ctx, config.Namespace)
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
				fnic.GCInterval = DefaultGCPeriod
				fnic.IdleDuraion = DefaultIdleDuraion
				return sharedcfg.RunnerFunc(func(stopC <-chan struct{}) {
					fnic.Run(1, stopC)
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
				xnc.GCInterval = DefaultGCPeriod
				xnc.IdleDuraion = DefaultIdleDuraion
				return sharedcfg.RunnerFunc(func(stopC <-chan struct{}) {
					xnc.Run(1, stopC)
				})
			})

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				r, err := funcinsts.NewOperator(
					cfg.RestConfig(),
					cfg.RefuncClient(),
					cfg.RefuncInformers(),
					natsbased.NewHandler(natsConn),
					credsyncer.NewSimpleProvider(),
				)
				if err != nil {
					klog.Fatalf("Failed to create operator, %v", err)
				}
				r.TappingInterval = 30 * time.Second

				return r
			})

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				r, err := crontrigger.NewOperator(
					cfg.Context(),
					cfg.RestConfig(),
					cfg.RefuncClient(),
					cfg.RefuncInformers(),
				)
				if err != nil {
					klog.Fatalf("Failed to create trigger, %v", err)
				}

				return r
			})

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				r, err := httptrigger.NewOperator(
					cfg.Context(),
					cfg.RestConfig(),
					cfg.RefuncClient(),
					cfg.RefuncInformers(),
				)
				if err != nil {
					klog.Fatalf("Failed to create trigger, %v", err)
				}

				r.CORS.AllowedOrigins = []string{"*"}
				r.CORS.AllowedMethods = []string{"GET", "HEAD", "POST"}
				r.CORS.AllowedHeaders = []string{"Accept", "Accept-Language", "Content-Language", "Origin"}
				r.CORS.AllowCredentials = true

				return r
			})

			var wg sync.WaitGroup
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
			wg.Wait()
		},
	}
	return cmd
}
