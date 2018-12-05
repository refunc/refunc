package sidecar

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"k8s.io/klog"

	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/messages"
)

// ReigsterHandlers register api handlers at given router
func (sc *Sidecar) reigsterHandlers(router *mux.Router) {
	apirouter := router.PathPrefix("/" + APIVersion).Subrouter()
	apirouter.Path("/ping").HandlerFunc(sc.handlePing).Methods(http.MethodGet)

	runtimerouter := apirouter.PathPrefix("/runtime").Subrouter()
	runtimerouter.Path("/invocation/next").HandlerFunc(sc.handleInvocationNext).Methods(http.MethodGet)
	runtimerouter.Path("/invocation/{rid}/response").HandlerFunc(sc.checkRequestID(sc.handleInvocationResponse)).Methods(http.MethodPost)
	runtimerouter.Path("/invocation/{rid}/error").Handler(sc.checkRequestID(sc.handleError)).Methods(http.MethodPost)
	runtimerouter.Path("/init/error").HandlerFunc(sc.handleError).Methods(http.MethodPost)
}

func (sc *Sidecar) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

func (sc *Sidecar) handleInvocationNext(w http.ResponseWriter, r *http.Request) {
	eng := sc.eng

	var request *messages.InvokeRequest

WAIT_LOOP:
	for {
		select {
		case <-r.Context().Done():
			klog.V(3).Info("(sc) connection closed by client")
			return

		case <-eng.NextC():
			request = eng.InvokeRequest()
			if request != nil {
				break WAIT_LOOP
			}
		}
	}

	deadline := request.Deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(messages.MaxTimeout)
	}

	// set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Lambda-Runtime-Aws-Request-Id", request.RequestID)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.FormatInt(deadline.UnixNano()/1e6, 10))
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", sc.fn.ARN())
	w.Header().Set("Lambda-Runtime-Trace-Id", request.TraceID)

	// w.Header().Set("Lambda-Runtime-Client-Context", xxx)
	// w.Header().Set("Lambda-Runtime-Cognito-Identity", xxx)

	w.Write(request.Args)
}

func (sc *Sidecar) handleInvocationResponse(w http.ResponseWriter, r *http.Request) {
	rid := mux.Vars(r)["rid"]
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "BodyReadError", err.Error())
		sc.eng.SetResult(rid, nil, err)
		return
	}

	if err := sc.eng.SetResult(rid, body, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err)
	} else {
		writeStatus(w, http.StatusAccepted, "OK")
	}
	w.(http.Flusher).Flush()
}

func (sc *Sidecar) handleError(w http.ResponseWriter, r *http.Request) {
	code, status := http.StatusAccepted, "OK"

	var lambdaErr messages.ErrorMessage
	body, err := ioutil.ReadAll(r.Body)

	if err != nil || json.Unmarshal(body, lambdaErr) != nil {
		code, status = 299, "InvalidErrorShape"
		klog.Errorf("(sidecar) handleInitError json error, %v", err)
	}

	errorType := r.Header.Get("Lambda-Runtime-Function-Error-Type")
	if errorType != "" {
		lambdaErr.Type = errorType
	}

	if rid := mux.Vars(r)["rid"]; rid != "" {
		if err := sc.eng.SetResult(rid, nil, lambdaErr); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			w.(http.Flusher).Flush()
			return
		}
	} else {
		lambdaErr.Fatal = true
		sc.eng.ReportInitError(lambdaErr)
	}

	writeStatus(w, code, status)
	w.(http.Flusher).Flush()
}

// TODO(bin)
func (sc *Sidecar) checkRequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
	}
}

func getErrorType(err interface{}) string {
	errorType := reflect.TypeOf(err)
	if errorType.Kind() == reflect.Ptr {
		return errorType.Elem().Name()
	}
	return errorType.Name()
}
