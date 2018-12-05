package wrapcobra

import (
	"os"

	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv"
	"github.com/spf13/cobra"
)

// Wrap will return a new cobra.Command, which will set
// flags using environment variable
func Wrap(cmd *cobra.Command) *cobra.Command {
	oriFn := cmd.Run
	cmd.Run = func(cmd *cobra.Command, args []string) {
		err := pflagenv.ParseSet(pflagenv.Prefix, cmd.Flags())
		if err != nil {
			cmd.Println("Error: ", err)
			return
		}
		// execute the original
		oriFn(cmd, args)
	}
	return cmd
}

func Execute(subCmdEnvSel string, cmd *cobra.Command) error {
	// select sub command from env
	if subCmdEnvSel == "" {
		subCmdEnvSel = "SUB_CMD"
	}
	cmdFromEnv := os.Getenv(subCmdEnvSel)
	if cmdFromEnv != "" && len(os.Args) == 1 {
		_, _, err := cmd.Find([]string{cmdFromEnv})
		if err != nil {
			return err
		}
		// override cmd
		os.Args = append(os.Args, cmdFromEnv)
	}

	return cmd.Execute()
}
