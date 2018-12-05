// modified from github.com/fission/fission
/*
Copyright 2016 The Fission Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mmux

import (
	"net/http"
	"sync/atomic"

	"github.com/gorilla/mux"
)

//
// MutableRouter wraps the mux router, and allows the router to be
// atomically changed.
type MutableRouter struct {
	router atomic.Value // mux.Router
}

// NewMutableRouter creates new mux with no handlers
func NewMutableRouter() *MutableRouter {
	return NewMutableRouterFromRouter(mux.NewRouter())
}

// NewMutableRouterFromRouter creates new mux from immutable parent
func NewMutableRouterFromRouter(router *mux.Router) *MutableRouter {
	mr := MutableRouter{}
	mr.router.Store(router)
	return &mr
}

// ServeHTTP implements the http.Handler interface
func (mr *MutableRouter) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	// Atomically grab the underlying mux router and call it.
	routerValue := mr.router.Load()
	router, ok := routerValue.(*mux.Router)
	if !ok {
		panic("Invalid router type")
	}
	router.ServeHTTP(responseWriter, request)
}

// UpdateRouter replaces the underlying mux with new one atomically
func (mr *MutableRouter) UpdateRouter(newHandler *mux.Router) {
	mr.router.Store(newHandler)
}
