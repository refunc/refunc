package httptrigger

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"k8s.io/klog"
)

var builtinPlugin = `
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func Event(req *http.Request) (interface{}, error) {
	argsMap := map[string]interface{}{}
	argsMap["$method"] = strings.ToLower(req.Method)
	argsMap["$header"] = req.Header
	argsMap["$remoteAddr"] = req.RemoteAddr
	argsMap["$requestURI"] = req.RequestURI
	if req.ContentLength > 0 {
		var (
			errInfo string
			err     error
		)
		ct := strings.ToLower(req.Header.Get("Content-Type"))
		switch ct {
		case "application/x-www-form-urlencoded":
			err = req.ParseForm()
			if err == nil {
				for k, v := range req.PostForm {
					argsMap[k] = v
				}
				return argsMap, nil
			}
			errInfo = fmt.Sprintf("x-www-form-urlencoded:%v", err)
		default:
			err = json.NewDecoder(req.Body).Decode(&argsMap)
			if err == nil {
				return argsMap, nil
			}
			errInfo = fmt.Sprintf("json:%v", err)
		}

		return argsMap, fmt.Errorf("%s", errInfo)
	} else {
		if err := req.ParseForm(); err != nil {
			return argsMap, err
		}
		for k, v := range req.Form {
			argsMap[k] = v
		}
		for k, v := range req.PostForm {
			argsMap[k] = v
		}
	}
	return argsMap, nil
}
`

type TriggerPlugins struct {
	plugins sync.Map
}

func NewTriggerPlugins() *TriggerPlugins {
	tp := &TriggerPlugins{}
	tp.installPlugin(".builtin", builtinPlugin)
	return tp
}

func (tp *TriggerPlugins) installPlugin(key, src string) error {
	if src == "" {
		return nil
	}

	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	if _, err := i.Eval(src); err != nil {
		return err
	}

	// event
	event, err := i.Eval("main.Event")
	if err != nil {
		return err
	}
	eventFunc, ok := event.Interface().(func(*http.Request) (interface{}, error))
	if !ok {
		return fmt.Errorf("plugin event func error")
	}
	tp.plugins.Store(fmt.Sprintf("%s.event", key), eventFunc)

	// more other plugin point

	klog.Infof("(httptrigger) install %s plugin", key)
	return nil
}

func (tp *TriggerPlugins) uninstallPlugin(key string) {
	_, loaded := tp.plugins.LoadAndDelete(fmt.Sprintf("%s.event", key))
	if loaded {
		klog.Infof("(httptrigger) uninstall %s plugin", key)
	}
}

func (tp *TriggerPlugins) reinstallPlugin(key string, src string) {
	tp.installPlugin(key, src)
}

func (tp *TriggerPlugins) loadPluginEvent(key string) (func(*http.Request) (interface{}, error), error) {
	eventFunc, ok := tp.plugins.Load(fmt.Sprintf("%s.event", key))
	if !ok {
		return tp.loadPluginEvent(".builtin")
	}
	return eventFunc.(func(*http.Request) (interface{}, error)), nil
}
