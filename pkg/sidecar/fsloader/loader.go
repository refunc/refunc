package fsloader

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog"

	"github.com/fsnotify/fsnotify"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/sidecar"
)

const (
	// ConfigFile if file name of config to load
	ConfigFile = "refunc.json"
	// DefaultPath default folder to search config file
	DefaultPath = "/var/run/refunc"
)

type loader struct {
	c    chan struct{}
	file string
	fn   *types.Function
}

func (l *loader) C() <-chan struct{} {
	return l.c
}

func (l *loader) Function() *types.Function {
	return l.fn
}

func (l *loader) loadConfig() (ok bool) {
	// TODO issue token for funcinst
	// defer func() {
	// 	os.Remove(l.file)
	// }()
	if _, err := os.Stat(l.file); os.IsNotExist(err) {
		return
	}
	bts, err := ioutil.ReadFile(l.file)
	if err != nil {
		klog.Warningf("(fsloader) failed to read config, %v", err)
		return
	}
	var fn types.Function
	if err := json.Unmarshal(bts, &fn); err != nil {
		klog.Warningf("(fsloader) failed to parse config, %v", err)
		return
	}
	l.fn = &fn
	klog.V(4).Infof("(fsloader) loaded from file %q", l.file)

	return true
}

// NewLoader creates a loader, watches given folder, and load refunc.yaml if any
func NewLoader(ctx context.Context, folder string) (sidecar.Loader, error) {
	l := new(loader)
	if folder == "" {
		folder = DefaultPath
	}
	l.file = filepath.Join(folder, ConfigFile)
	if l.loadConfig() {
		l.c = closedSigChan
		return l, nil
	}

	// file does not ready, start a watcher
	l.c = make(chan struct{})
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Errorf("(fsloader) failed to create watcher, %v", err)
		return nil, err
	}
	err = watcher.Add(folder)
	if err != nil {
		klog.Errorf("(fsloader) failed to watch, %v", err)
		watcher.Close()
		return nil, err
	}
	// start watcher
	go func() {
		defer close(l.c)
		defer watcher.Close()
		wanted := fsnotify.Write | fsnotify.Create
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// verbose for debug
				if klog.V(4) {
					klog.V(4).Infof("(fsloader) recived event: %q", func() string {
						if event.Op&fsnotify.Create == fsnotify.Create {
							return "Create: " + event.Name
						}
						return "Write: " + event.Name
					}())
				}
				if event.Op&wanted != 0x0 && strings.HasSuffix(event.Name, ConfigFile) && l.loadConfig() {
					return
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				klog.Warningf("(fsloader) failed to watching config, %v", err)
			}
		}
	}()

	return l, nil
}

var closedSigChan = make(chan struct{})

func init() {
	close(closedSigChan)
}
