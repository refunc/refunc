package main

import (
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"runtime"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/runtime/refunc/loader"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/flagtools"
	"github.com/spf13/pflag"
)

var config struct {
	WorkDir string
	Loader  string
	Addr    string

	AccessKey   string
	HostKeyFile string
}

func init() {
	pflag.StringVarP(&config.WorkDir, "base", "b", cwd(), "The root of working dir")
	pflag.StringVarP(&config.Addr, "listen", "l", ":7788", "The listen address")
	pflag.StringVarP(&config.Loader, "loader", "a", "", "The path of the addin to load a func")
	pflag.StringVarP(&config.AccessKey, "access-key", "k", "", "The access key for controller to login")
	pflag.StringVar(&config.HostKeyFile, "host-key", "", "The path for host key file")

	pflag.StringVarP(&config.Loader, "entry", "e", "", "The default entry to load script")
	pflag.CommandLine.MarkDeprecated("entry", "using --loader instead")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())

	flagtools.InitFlags()

	klog.CopyStandardLogTo("INFO")
	defer klog.Flush()

	if config.AccessKey == "" {
		klog.Fatal("password must not be empty")
	}

	hostKeyPEM, err := ioutil.ReadFile(config.HostKeyFile)
	if err != nil {
		if config.HostKeyFile == "" {
			klog.Warning("using default host key")
		} else {
			klog.Warningf("failed to read host key file, %v", err)
		}
	}

	if config.WorkDir == "" {
		config.WorkDir = cwd()
	}

	if config.Loader == "-" {
		klog.Info("using builtin addin(native command opener)")
		config.Loader = ""
	}

	ld := &loader.Agent{
		WorkDir:    config.WorkDir,
		Loader:     config.Loader,
		AccessKey:  config.AccessKey,
		HostKeyPEM: hostKeyPEM,
	}

	go func() {
		err := ld.Server(config.Addr)
		if err != nil {
			klog.Fatal(err)
		}
	}()

	klog.Infof(`received signal "%v", exiting...`, <-cmdutil.GetSysSig())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	klog.Infof("agent shutting down %v", func() string {
		if err := ld.Shutdown(ctx); err != nil {
			return "with error, " + err.Error()
		}
		return "without error."
	}())
}

func cwd() (wd string) {
	wd, _ = os.Getwd()
	return
}
