package sidecar

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"syscall"
	"time"

	"k8s.io/klog"

	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

// ReigsterHandlers register api handlers at given router
func (sc *Sidecar) reigsterHandlers(router *mux.Router) {
	apirouter := router.PathPrefix("/" + APIVersion).Subrouter()
	apirouter.Path("/ping").HandlerFunc(sc.handlePing).Methods(http.MethodGet)
	apirouter.Path("/{wid}/log").HandlerFunc(sc.handleLog).Methods(http.MethodGet)

	runtimerouter := apirouter.PathPrefix("/runtime").Subrouter()
	runtimerouter.Path("/invocation/next").HandlerFunc(sc.handleInvocationNext).Methods(http.MethodGet)
	runtimerouter.Path("/invocation/{rid}/response").HandlerFunc(sc.checkRequestID(sc.handleInvocationResponse)).Methods(http.MethodPost)
	runtimerouter.Path("/invocation/{rid}/error").Handler(sc.checkRequestID(sc.handleError)).Methods(http.MethodPost)
	runtimerouter.Path("/init/error").HandlerFunc(sc.handleError).Methods(http.MethodPost)
}

func (sc *Sidecar) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong")) //nolint:errcheck
}

func (sc *Sidecar) handleLog(w http.ResponseWriter, r *http.Request) {
	wid := mux.Vars(r)["wid"]

	pipeFile := fmt.Sprintf("%s/%s.log.pipe", RefuncRoot, wid)
	if err := syscall.Mkfifo(pipeFile, 0666); err != nil {
		w.Write([]byte("error"))
		return
	}

	fd, err := os.OpenFile(pipeFile, os.O_RDWR, fs.ModeNamedPipe)
	if err != nil {
		w.Write([]byte("error"))
		return
	}

	sc.logStreams.Store(wid, fd)
	klog.Infof("(car) handle log stream %s", wid)

	go func() {
		defer sc.logStreams.Delete(wid)

		var buf [4096]byte
		for {
			n, err := fd.Read(buf[:])
			if err != nil {
				klog.Errorf("(car) handle log read faild %v", err)
				return
			}
			sc.eng.WriteLog(wid, buf[:n])

			// previous request log have write out completed?
			// forward log is useful to debug?
			if forwardEndpoint, ok := sc.logForwards.Load(wid); ok {
				sc.eng.ForwardLog(forwardEndpoint.(string), buf[:n])
			}
		}
	}()

	w.Write([]byte(pipeFile))
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

	wid := r.Header.Get("Refunc-Worker-ID")
	if wid != "" && request.Options != nil {
		if forwardLogEndpoint, ok := request.Options["logEndpoint"]; ok {
			sc.logForwards.Store(wid, forwardLogEndpoint)
		}
	}

	deadline := request.Deadline
	if deadline.IsZero() {
		// FIXME (bin)
		// potential long running task?
		deadline = time.Now().Add(24 * 365 * 10 * time.Hour)
	}
	klog.V(3).Infof("(car) on reqeust %s with deadline %s", request.RequestID, deadline.Format(time.RFC3339))

	// set headers
	if value, ok := request.Options["content-type"]; ok {
		w.Header().Set("Content-Type", value.(string))
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("Lambda-Runtime-Aws-Request-Id", request.RequestID)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.FormatInt(deadline.UnixNano()/1e6, 10))
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", sc.fn.ARN())
	w.Header().Set("Lambda-Runtime-Trace-Id", request.TraceID)

	// w.Header().Set("Lambda-Runtime-Client-Context", xxx)
	// w.Header().Set("Lambda-Runtime-Cognito-Identity", xxx)

	w.Write(request.Args) //nolint:errcheck
}

func (sc *Sidecar) handleInvocationResponse(w http.ResponseWriter, r *http.Request) {
	rid := mux.Vars(r)["rid"]
	body, err := ioutil.ReadAll(r.Body)
	contentType := r.Header.Get("Content-Type")
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "BodyReadError", err.Error())
		if err := sc.eng.SetResult(rid, nil, err, contentType); err != nil {
			klog.Errorf("(car) failed set result, %v", err)
		}
		return
	}

	klog.V(3).Infof("(sidecar) on response %s - %v", rid, utils.ByteSize(uint64(len(body))))
	if err := sc.eng.SetResult(rid, body, nil, contentType); err != nil {
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

	if err != nil {
		code, status = 299, "InvalidErrorShape"
		lambdaErr = *messages.GetErrorMessage(err)
	} else if err := json.Unmarshal(body, &lambdaErr); err != nil {
		code, status = 299, "InvalidErrorShape"
		lambdaErr = *messages.GetErrorMessage(err)
		klog.Errorf("(sidecar) handleInitError json error, %v, %v", err, string(body))
	}

	errorType := r.Header.Get("Lambda-Runtime-Function-Error-Type")
	if errorType != "" {
		lambdaErr.Type = errorType
	}

	klog.V(3).Infof("(sidecar) on error, %v", lambdaErr)
	if rid := mux.Vars(r)["rid"]; rid != "" {
		if err := sc.eng.SetResult(rid, nil, lambdaErr, r.Header.Get("Content-Type")); err != nil {
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

func (sc *Sidecar) checkRequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wid := r.Header.Get("Refunc-Worker-ID")
		if wid != "" {
			sc.logForwards.Delete(wid)
		}
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
