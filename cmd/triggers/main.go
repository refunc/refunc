package triggers

import (
	"context"
	"os"
	"sync"

	"k8s.io/klog"

	nats "github.com/nats-io/go-nats"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv/wrapcobra"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"
)

// well known default constants
const (
	EnvMyPodName      = "REFUNC_NAME"
	EnvMyPodNamespace = "REFUNC_NAMESPACE"
)

type triggerOperator interface {
	Run(stop <-chan struct{})
}

type operatorConfig struct {
	sharedcfg.SharedConfigs
	NatsConn *nats.Conn
}

// NewCmd creates new commands
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "the function triggers",
		Run: func(cmd *cobra.Command, args []string) {
			// print commands' help
			cmd.Help()
		},
	}
	cmd.AddCommand(wrapcobra.Wrap(cmdRPCTrigger()))
	cmd.AddCommand(wrapcobra.Wrap(cmdCronTrigger()))
	return cmd
}

func triggerCmdTemplate(factory func(config *operatorConfig)) *cobra.Command {
	var config struct {
		Namespace string
	}

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			sc := sharedcfg.New(ctx, config.Namespace)

			natsConn, err := env.NewNatsConn(nats.Name(os.Getenv(EnvMyPodNamespace) + "/" + os.Getenv(EnvMyPodName)))
			if err != nil {
				klog.Fatalf("Failed to connect to nats %s, %v", env.GlobalNatsURLString(), err)
			}
			defer natsConn.Close()

			factory(&operatorConfig{
				NatsConn:      natsConn,
				SharedConfigs: sc,
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
	cmd.Flags().StringVarP(&config.Namespace, "namespace", "n", "", "The scope of namepsace to manipulate")

	return cmd
}
