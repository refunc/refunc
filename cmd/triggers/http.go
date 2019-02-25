package triggers

import (
	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/operators/triggers/httptrigger"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/cobra"
)

func cmdRPCTrigger() *cobra.Command {
	var config struct {
		CORS struct {
			AllowedMethods []string
			AllowedHeaders []string
			AllowedOrigins []string
			ExposedHeaders []string

			MaxAge           int
			AllowCredentials bool
		}
	}

	cmd := triggerCmdTemplate(func(sc sharedcfg.SharedConfigs) {
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

			// config
			r.CORS = config.CORS

			return r
		})
	})

	cmd.Use = "http"
	cmd.Short = "http trigger for function"
	cmd.Long = cmd.Short

	// cors
	cmd.Flags().StringSliceVar(&config.CORS.AllowedOrigins, "cors-allowed-origins", []string{}, "CORS config for allowed origins")
	cmd.Flags().StringSliceVar(&config.CORS.AllowedMethods, "cors-allowed-methods", []string{}, "CORS config for allowed methods")
	cmd.Flags().StringSliceVar(&config.CORS.AllowedHeaders, "cors-allowed-headers", []string{}, "CORS config for allowed headers")
	cmd.Flags().StringSliceVar(&config.CORS.ExposedHeaders, "cors-exposed-headers", []string{}, "CORS config for exposed headers")
	cmd.Flags().BoolVar(&config.CORS.AllowCredentials, "cors-allow-credentials", false, "CORS config if allow credentials")
	cmd.Flags().IntVar(&config.CORS.MaxAge, "cors-max-age", 0, "CORS config for max age")

	return cmd
}
