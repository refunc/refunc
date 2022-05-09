package sidecar

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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
	go sc.tailLog(wid, fd)

	w.Write([]byte(pipeFile))
}

var LogFrameDelimer = []byte{165, 90, 0, 1} //0xA55A0001

func (sc *Sidecar) tailLog(wid string, fd io.Reader) {
	logEndpoint := fmt.Sprintf("%s.%s", sc.fn.Spec.Runtime.Envs["REFUNC_LOG_ENDPOINT"], wid)

	klog.Infof("(car) tail log stream %s", wid)

	defer sc.logStreams.Delete(wid)

	decodeLog := func(data []byte) (string, []byte) {
		var offset uint32 = 0
		var offlen uint32 = 4
		endpointLen := binary.BigEndian.Uint32(data[offset : offset+offlen])
		endpoint := string(data[offset+offlen : offset+offlen+endpointLen])

		offset = offset + offlen + endpointLen
		payloadLen := binary.BigEndian.Uint32(data[offset : offset+offlen])
		payload := data[offset+offlen : offset+offlen+payloadLen]
		return endpoint, payload
	}

	scanner := bufio.NewScanner(fd)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if i := bytes.Index(data, LogFrameDelimer); i >= 0 {
			if len(data[:i]) == 0 {
				return i + len(LogFrameDelimer), nil, nil
			}
			return i + len(LogFrameDelimer), data[:i], nil
		}
		if atEOF {
			return 0, data, bufio.ErrFinalToken
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		bts := scanner.Bytes()
		forward, msg := decodeLog(bts)
		if forward != "" {
			sc.eng.ForwardLog(forward, msg)
		}
		sc.logger.WriteLog(logEndpoint, msg)
	}
	if err := scanner.Err(); err != nil {
		klog.Errorf("(car) tail log read faild %s %v", wid, err)
	}

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

	if request.Options != nil {
		if forwardLogEndpoint, ok := request.Options["logEndpoint"]; ok {
			w.Header().Set("Lambda-Runtime-Forward-Log-Endpoint", forwardLogEndpoint.(string))
		}
	}

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
