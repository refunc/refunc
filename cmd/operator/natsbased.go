package operator

import (
	"os"
	"time"

	"k8s.io/klog"

	nats "github.com/nats-io/nats.go"
	"github.com/refunc/refunc/pkg/credsyncer"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/operators/funcinsts"
	"github.com/refunc/refunc/pkg/transport/natsbased"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"
)

// We should start tapping active services
const defaultTappingInterval = 30 * time.Second

func cmdNatsBased() *cobra.Command {
	var config struct {
		TappingInterval time.Duration
	}

	cmd := operatorCmdTemplate(func(cfg sharedcfg.Configs) sharedcfg.Runner {
		natsConn, err := env.NewNatsConn(nats.Name(os.Getenv(EnvMyPodNamespace) + "/" + os.Getenv(EnvMyPodName)))
		if err != nil {
			klog.Fatalf("Failed to connect to nats %s, %v", env.GlobalNatsURLString(), err)
		}

		r, err := funcinsts.NewOperator(
			cfg.RestConfig(),
			cfg.RefuncClient(),
			cfg.RefuncInformers(),
			natsbased.NewHandler(natsConn),
			credsyncer.NewGeneratedProvider(24*time.Hour),
		)
		if err != nil {
			natsConn.Close()
			klog.Fatalf("Failed to create trigger, %v", err)
		}
		r.TappingInterval = config.TappingInterval

		return sharedcfg.RunnerFunc(func(stopC <-chan struct{}) {
			defer natsConn.Close()
			r.Run(stopC)
		})
	})

	cmd.Use = "nats"
	cmd.Short = "operator for basic event trigger"
	cmd.Long = cmd.Short

	cmd.Flags().DurationVar(&config.TappingInterval, "tapping-interval", defaultTappingInterval, "The interval bewteen each tapping (should at max half of refunc's lifetime)")

	return cmd
}
