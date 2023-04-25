package loader

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/loader/fsloader"
	"github.com/refunc/refunc/pkg/runtime/types"
)

// Loader listens to address to load res and handles the bootstrap of a func
type Loader interface {
	Start(ctx context.Context) error
}

// NewSimpleLoader creates a new simple loader,
// simple loader will listen POST reqeust at /init, and download resouces
func NewSimpleLoader(main, taskRoot, runtimeRoot, layersRoot string) Loader {
	return &simpleLoader{
		Main:        main,
		TaskRoot:    taskRoot,
		RuntimeRoot: runtimeRoot,
		LayersRoot:  layersRoot,
	}
}

type simpleLoader struct {
	Main string

	TaskRoot    string
	RuntimeRoot string
	LayersRoot  string

	ctx context.Context
}

// common vars
var (
	RefuncRoot = "/var/run/refunc"
	ConfigFile = "refunc.json"

	DefaultMain        = "/var/runtime/bootstrap"
	AlterMainPath      = "/opt/bootstrap"
	DefaultTaskRoot    = "/var/task"
	DefaultRuntimeRoot = "/var/runtime"
	DefaultLayersRoot  = "/opt"
)

func (ld *simpleLoader) Start(ctx context.Context) error {
	ld.ctx = ctx
	fn, err := ld.loadFunc()
	if err != nil {
		klog.Infof("(loader) cannot load function, %v", err)
		if fn, err = ld.wait(ctx, RefuncRoot); err != nil {
			return err
		}
	}
	return ld.exec(fn)
}

func (ld *simpleLoader) exec(fn *types.Function) error {
	if apiAddr := fn.Spec.Runtime.Envs["AWS_LAMBDA_RUNTIME_API"]; apiAddr != "" {
		klog.Infoln("(loader) ping api")
		for i := 0; i < 200; i++ {
			res, err := http.Get("http://" + apiAddr + "/2018-06-01/ping")
			if err != nil {
				select {
				case <-ld.ctx.Done():
					return ld.ctx.Err()
				case <-time.After(5 * time.Millisecond):
					continue
				}
			}
			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil || string(body) != "pong" {
				return errors.New("loader: failed to reqeust api")
			}
			break
		}
	}

	concurrency := withConcurrency(fn)
	cmds := []*exec.Cmd{}
	for i := 0; i < concurrency; i++ {
		cmd, err := ld.prepare(fn)
		if err != nil {
			return err
		}
		cmds = append(cmds, cmd)
	}

	wg := sync.WaitGroup{}
	for i, cmd := range cmds {
		wg.Add(1)
		go func(wid int, c *exec.Cmd) {
			klog.Infof("(loader) worker #%d exec %s", wid, strings.Join(c.Args, " "))
			if err := c.Run(); err != nil {
				klog.Errorf("(loader) worker #%d exec %s error %v", wid, strings.Join(c.Args, " "), err)
			}
			wg.Done()
		}(i, cmd)
	}
	wg.Wait()

	return errors.New("(loader) all workers exit")
}

func (ld *simpleLoader) wait(ctx context.Context, folder string) (*types.Function, error) {
	fnld, err := fsloader.NewLoader(ctx, folder)
	if err != nil {
		return nil, err
	}
	<-fnld.C()
	fn := fnld.Function()
	if fn == nil {
		return nil, errors.New("(loader) wait funcdef error")
	}
	ld.setup(fn)
	return fn, nil
}

func (ld *simpleLoader) mainExe() string {
	if ld.Main != "" {
		return ld.Main
	}
	return DefaultMain
}

func (ld *simpleLoader) taskRoot() string {
	if ld.TaskRoot != "" {
		return ld.TaskRoot
	}
	return DefaultTaskRoot
}

func (ld *simpleLoader) runtimeRoot() string {
	if ld.RuntimeRoot != "" {
		return ld.RuntimeRoot
	}
	return DefaultRuntimeRoot
}

func (ld *simpleLoader) layersRoot() string {
	if ld.LayersRoot != "" {
		return ld.LayersRoot
	}
	return DefaultLayersRoot
}
