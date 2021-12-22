package operator

import (
	"context"
	"sync"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/credsyncer/verifier"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv/wrapcobra"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"

	// load builtins
	_ "github.com/refunc/refunc/pkg/builtins/helloworld"
	_ "github.com/refunc/refunc/pkg/builtins/sign"
)

// well known default constants
const (
	EnvMyPodName      = "REFUNC_NAME"
	EnvMyPodNamespace = "REFUNC_NAMESPACE"
)

var config struct {
	Namespace string
}

// NewCmd creates new commands
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "operator of functinsts for different trasnport",
		Run: func(cmd *cobra.Command, args []string) {
			// print commands' help
			cmd.Help()
		},
	}
	cmd.AddCommand(wrapcobra.Wrap(cmdNatsBased()))
	cmd.PersistentFlags().StringVarP(&config.Namespace, "namespace", "n", "", "The scope of namepsace to manipulate")
	return cmd
}

func operatorCmdTemplate(factory func(cfg sharedcfg.Configs) sharedcfg.Runner) *cobra.Command {

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			sc := sharedcfg.New(ctx, config.Namespace)

			sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
				r := factory(cfg)
				// nolint:errcheck init buildin verifier
				verifier.RegisterVerifer(
					cfg.Context().Done(),
					cfg.Namespace(),
					cfg.RefuncInformers(),
					cfg.KubeInformers(),
				)
				return r
			})

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				sc.Run(ctx.Done())
			}()

			klog.Infof(`Received signal "%v", exiting...`, <-cmdutil.GetSysSig())

			cancel()
			wg.Wait()
		},
	}

	return cmd
}
