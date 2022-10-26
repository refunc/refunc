package main

import (
	"context"
	"math/rand"
	"runtime"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/logger"
	"github.com/refunc/refunc/pkg/runtime/lambda/loader"
	"github.com/refunc/refunc/pkg/sidecar"
	"github.com/refunc/refunc/pkg/sidecar/fsloader"
	"github.com/refunc/refunc/pkg/transport/natsbased/natscar"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/flagtools"
	"github.com/spf13/pflag"
)

var config struct {
	Addr         string
	RefuncRoot   string
	Logger       string
	LoggerConfig string
}

func init() {
	pflag.StringVarP(&config.Addr, "listen", "l", "127.0.0.1:80", "The listen address")
	pflag.StringVar(&config.RefuncRoot, "refunc-root", loader.RefuncRoot, "The root of layers folder")
	pflag.StringVar(&config.Logger, "logger", "stdout", "The logger of func logging")
	pflag.StringVar(&config.LoggerConfig, "logger-config", "", "The logger config of func logging")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())

	flagtools.InitFlags()

	klog.CopyStandardLogTo("INFO")
	defer klog.Flush()

	if config.RefuncRoot == "" && config.RefuncRoot != fsloader.DefaultPath {
		config.RefuncRoot = fsloader.DefaultPath
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ld, err := fsloader.NewLoader(ctx, config.RefuncRoot)
	if err != nil {
		klog.Exitf("Failed to creats loader, %v", err)
	}

	logger, err := logger.CreateLogger(ctx, config.Logger, config.LoggerConfig)
	if err != nil {
		klog.Exitf("Failed to create logger, %v", err)
	}

	eng := natscar.NewEngine()

	car := sidecar.NewCar(eng, ld, logger)

	go func() {
		klog.Infof(`received signal "%v", exiting...`, <-cmdutil.GetSysSig())
		cancel()
	}()

	car.Serve(ctx, config.Addr)
}
