package main

import (
	"context"
	"math/rand"
	"runtime"
	"strings"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/runtime/lambda/loader"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/flagtools"
	"github.com/spf13/pflag"
)

var config struct {
	Addr string

	TaskRoot    string
	RuntimeRoot string
	LayersRoot  string

	RefuncRoot string
}

func init() {
	pflag.StringVarP(&config.Addr, "listen", "l", ":7788", "The listen address")
	pflag.StringVar(&config.TaskRoot, "task-root", loader.DefaultTaskRoot, "The root of task folder")
	pflag.StringVar(&config.RuntimeRoot, "runtime-root", loader.DefaultRuntimeRoot, "The root of runtime folder")
	pflag.StringVar(&config.LayersRoot, "layers-root", loader.DefaultLayersRoot, "The root of layers folder")

	pflag.StringVar(&config.RefuncRoot, "refunc-root", loader.RefuncRoot, "The root of layers folder")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())

	flagtools.InitFlags()

	klog.CopyStandardLogTo("INFO")
	defer klog.Flush()

	if config.RefuncRoot != "" && config.RefuncRoot != loader.RefuncRoot {
		klog.Infof("refunc root is set to: %s", config.RefuncRoot)
		loader.RefuncRoot = config.RefuncRoot
	}

	ld := loader.NewSimpleLoader(strings.Join(pflag.Args(), " "), config.TaskRoot, config.RuntimeRoot, config.LayersRoot)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		klog.Infof(`received signal "%v", exiting...`, <-cmdutil.GetSysSig())
		cancel()
	}()

	err := ld.Start(ctx, config.Addr)
	if err != nil {
		klog.Error(err)
	}
}
