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
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// nolint:errcheck
func OldHandler(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Write([]byte("old handler"))
}

// nolint:errcheck
func NewHandler(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Write([]byte("new handler"))
}

func verifyRequest(expectedResponse string) {
	targetURL := "http://localhost:3333"
	testRequest(targetURL, expectedResponse)
}

func startServer(mr *MutableRouter) {
	http.ListenAndServe(":3333", mr) // nolint:errcheck
}

func spamServer(quit chan bool) {
	i := 0
	for {
		select {
		case <-quit:
			break
		default:
			i = i + 1
			resp, err := http.Get("http://localhost:3333")
			if err != nil {
				log.Panicf("failed to make get request %v: %v", i, err)
			}
			resp.Body.Close()
		}
	}
}

func TestMutableMux(t *testing.T) {
	// make a simple mutable router
	log.Print("Create mutable router")
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", OldHandler)
	mr := NewMutableRouterFromRouter(muxRouter)

	// start http server
	log.Print("Start http server")
	go startServer(mr)

	// continuously make requests, panic if any fails
	time.Sleep(100 * time.Millisecond)
	q := make(chan bool)
	go spamServer(q)

	time.Sleep(5 * time.Millisecond)

	// connect and verify old handler
	log.Print("Verify old handler")
	verifyRequest("old handler")

	// change the muxer
	log.Print("Change mux router")
	newMuxRouter := mux.NewRouter()
	newMuxRouter.HandleFunc("/", NewHandler)
	mr.UpdateRouter(newMuxRouter)

	// connect and verify the new handler
	log.Print("Verify new handler")
	verifyRequest("new handler")

	q <- true
	time.Sleep(100 * time.Millisecond)
}

func testRequest(targetURL string, expectedResponse string) {
	resp, err := http.Get(targetURL)
	if err != nil {
		log.Panicf("failed to make get request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Panicf("response status: %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Panic("failed to read response")
	}

	bodyStr := string(body)
	log.Printf("Server responded with %v", bodyStr)
	if bodyStr != expectedResponse {
		log.Panic("Unexpected response")
	}
}
