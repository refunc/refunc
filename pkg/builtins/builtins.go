package builtins

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/refunc/refunc/pkg/messages"
)

// RequestHandler handler of request
type RequestHandler func(request *messages.InvokeRequest) (reply interface{}, err error)

// store handlers and avoid concurrent panic
var handlerMap sync.Map

type handlerValue struct {
	meta    string
	handler RequestHandler
}

// RegisterBuiltin register a handler for builtins funcs
func RegisterBuiltin(name string, meta string, handler RequestHandler) {
	handlerMap.Store(name, &handlerValue{
		meta:    meta,
		handler: handler,
	})
}

// ListBuiltins return a list name of handlers
func ListBuiltins() (names []string) {
	handlerMap.Range(func(key interface{}, value interface{}) bool {
		names = append(names, key.(string))
		return true
	})
	return
}

var errFuncNotFound = errors.New("builtins: func not found")

// HandleBuiltins handles build request
func HandleBuiltins(name string, raw []byte, reply func([]byte, error)) {
	val, has := handlerMap.Load(name)
	if !has {
		reply(nil, errFuncNotFound)
		return
	}
	var req messages.InvokeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		reply(nil, err)
		return
	}
	result, err := val.(*handlerValue).handler(&req)
	if err != nil {
		reply(nil, err)
		return
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		reply(nil, err)
		return
	}
	reply([]byte(fmt.Sprintf(`{"a":"rsp","p":%s}`, resultBytes)), nil)
}
