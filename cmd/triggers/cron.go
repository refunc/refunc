package triggers

import (
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/operators/triggers/crontrigger"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"
)

func cmdCronTrigger() *cobra.Command {

	var config struct {
		Offset time.Duration
	}

	cmd := triggerCmdTemplate(func(sc sharedcfg.SharedConfigs) {
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

			// config
			if config.Offset > 0 {
				klog.Infof("cron offset %v", config.Offset)
				r.Offset = config.Offset
			}

			return r
		})
	})

	cmd.Use = "cron"
	cmd.Short = "operator for cron trigger"
	cmd.Long = cmd.Short

	cmd.Flags().DurationVar(&config.Offset, "offset", 0, "offset adds to time when calc next fire time")

	return cmd
}
