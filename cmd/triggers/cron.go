package triggers

import (
	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/operators/triggers/crontrigger"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"
)

func cmdCronTrigger() *cobra.Command {

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

			return r
		})
	})

	cmd.Use = "cron"
	cmd.Short = "operator for cron trigger"
	cmd.Long = cmd.Short

	return cmd
}
