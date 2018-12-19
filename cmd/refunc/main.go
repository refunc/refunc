package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/cmd/controller"
	"github.com/refunc/refunc/cmd/play"
	"github.com/refunc/refunc/cmd/triggers"
	"github.com/refunc/refunc/pkg/utils/cmdutil/flagtools"
	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv"
	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv/wrapcobra"
	"github.com/refunc/refunc/pkg/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	// config runtimes and transport
	_ "github.com/refunc/refunc/pkg/runtime/lambda/runtime"
	_ "github.com/refunc/refunc/pkg/runtime/refunc/runtime"
)

func main() {
	runtime.GOMAXPROCS(16 * runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())

	klog.CopyStandardLogTo("INFO")
	defer klog.Flush()

	cmd := &cobra.Command{
		Use:   "refunc",
		Short: "refunc is a bundle of refunc's compoents",
		Run: func(cmd *cobra.Command, args []string) {
			// print commands' help
			cmd.Help()
		},
	}

	bindFlags(cmd.PersistentFlags())
	// set global flags using env
	pflagenv.ParseSet(pflagenv.Prefix, cmd.PersistentFlags())

	cmd.AddCommand(wrapcobra.Wrap(controller.NewCmd()))
	cmd.AddCommand(wrapcobra.Wrap(triggers.NewCmd()))
	cmd.AddCommand(wrapcobra.Wrap(play.NewCmd()))

	// version command
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "show current version of refunc",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("refunc: " + version.Version)
			os.Exit(0)
		},
	})

	// select sub command from env
	if err := cmd.Execute(); err != nil {
		klog.Fatal(err)
	}
}

// bindFlags adds any flags that are common to all redatacli sub commands.
func bindFlags(flags *pflag.FlagSet) {
	flagset := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(flagset)

	// any flags defined by external projects (not part of pflags)
	flagtools.AddFlagSetToPFlagSet(flagset, flags)
	// set default flags, enable klog.Set*Ouput to capture logs
	flagset.Set("alsologtostderr", "true")
	flagset.Set("log_file", "/dev/null")

	// Normalize all flags that are coming from other packages or pre-configurations
	// a.k.a. change all "_" to "-". e.g. glog package
	flags.SetNormalizeFunc(flagtools.WordSepNormalizeFunc)

	// hack for glog
	flag.Set("logtostderr", "true")
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	os.Args = []string{os.Args[0]}
	flag.Parse()
	os.Args = oldArgs

}
