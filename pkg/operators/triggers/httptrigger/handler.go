package httptrigger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog"

	"github.com/gorilla/mux"
	observer "github.com/refunc/go-observer"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

var (
	emptyProp     = observer.NewProperty(nil)
	blockTickerCh = make(chan time.Time)
)

const (
	jsonCT = "application/json; charset=utf-8"
)

type httpHandler struct {
	fndKey string
	ns     string
	name   string

	operator *Operator
}

func (t *httpHandler) setupHTTPEndpoints(router *mux.Router) {
	base := "/" + t.fndKey

	// subrouter prefixed with /ns/refunc-name
	sr := router.PathPrefix(base).Subrouter()

	const (
		streamingOn  = true
		streamingOff = false
	)

	// POST /ns.refunc-name
	// create a new invocation
	router.HandleFunc(base, t.taskCreationHandler(streamingOff))

	// POST /ns/refunc-name/
	// create a new task
	sr.HandleFunc("/", t.taskCreationHandler(streamingOff))

	// GET /ns/refunc-name/_meta
	// query metadata of the refunc
	sr.HandleFunc("/_meta", t.handleMeta).Methods(http.MethodGet)
}

func (t *httpHandler) taskCreationHandler(streaming bool) func(http.ResponseWriter, *http.Request) {

	return func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			if re := recover(); re != nil {
				utils.LogTraceback(re, 4, klog.V(1))
				writeHTTPError(rw, http.StatusInternalServerError, fmt.Sprintf("%v", re))
			}
		}()

		trigger, err := t.operator.TriggerLister.Triggers(t.ns).Get(t.name)
		if err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}
		fndef, err := t.operator.ResolveFuncdef(trigger)
		if err != nil {
			if k8sutil.IsResourceNotFoundError(err) {
				writeHTTPError(rw, http.StatusNotFound, err.Error())
			} else {
				writeHTTPError(rw, http.StatusBadRequest, err.Error())
			}
			return
		}
		if fndef.Namespace != t.ns {
			writeHTTPError(rw, http.StatusBadRequest, `h: invoke function in other namespace is not allowed`)
			return
		}

		if req.ContentLength > messages.MaxPayloadSize {
			writeHTTPError(rw, http.StatusBadRequest, `exceed max payload size limit`)
			return
		}

		// parse payload
		args, err := GetPayload(req)
		if err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}

		data, err := SortArgs(args)
		if err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}

		// insert meta keys
		var argsMap map[string]interface{}
		if err := json.Unmarshal(data, &argsMap); err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}

		if argsMap["$request"], err = GetRequest(req, args); err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}
		argsMap["$method"] = strings.ToLower(req.Method)
		data = messages.MustFromObject(argsMap)

		// create request
		rid := GetRequestID(req)
		id := path.Join(t.fndKey, rid)

		request := &messages.InvokeRequest{
			Args:      data,
			RequestID: rid,
			Options: map[string]interface{}{
				"method": strings.ToLower(req.Method),
			},
		}

		// get TaskResolver
		taskr, err := t.ensureTask(fndef.DeepCopy(), trigger.DeepCopy(), request)
		if err != nil {
			klog.Errorf("(h) %s failed to start task, %v", id, err)
			writeHTTPError(rw, http.StatusInternalServerError, err.Error())
			return
		}

		ctx := req.Context()
		isWeb := false
		if trigger.Spec.HTTPTrigger != nil {
			isWeb = trigger.Spec.HTTPTrigger.Web
		}
		t.taskPoller(ctx, rw, isWeb, taskr, blockTickerCh, false)()
	}
}

func (t *httpHandler) handleMeta(rw http.ResponseWriter, req *http.Request) {
	defer func() {
		if re := recover(); re != nil {
			utils.LogTraceback(re, 4, klog.V(1))
			writeHTTPError(rw, http.StatusInternalServerError, fmt.Sprintf("%v", re))
		}
	}()

	// TODO (bin): maybe using annotations
	// serve embeded meta
	rw.Header().Set("Content-Type", jsonCT)
	rw.Write(append([]byte{'{', '}'}, messages.TokenCRLF...)) // nolint:errcheck
}

func (t *httpHandler) flushWriter(rw http.ResponseWriter, idH string) func([]byte) bool {
	return func(bts []byte) bool {
		_, err := rw.Write(bts)
		if err != nil {
			klog.Errorf("(h) %s failed to write, %v", idH, err)
			return false
		}
		flushRW(rw)
		return true
	}
}

func (t *httpHandler) taskPoller(
	ctx context.Context,
	rw http.ResponseWriter,
	web bool,
	taskr client.TaskResolver,
	tickerC <-chan time.Time,
	fwdlog bool,
) func() bool {
	var logsteam observer.Stream
	if fwdlog {
		logsteam = taskr.LogObserver()
	}
	if logsteam == nil {
		logsteam = emptyProp.Observe()
	}
	write := t.flushWriter(rw, taskr.ID())

	return func() bool {
		select {
		case <-ctx.Done():
			klog.V(3).Infof("(h) %s, %v", taskr.ID(), ctx.Err())
			write(messages.GetErrActionBytes(ctx.Err()))

		case <-tickerC:
			// write ping
			return write(messages.PingMsg)

		case <-logsteam.Changes():
			var lines []byte
			for logsteam.HasNext() {
				logline := logsteam.Next().(string)
				if us, err := strconv.Unquote(`"` + logline + `"`); err == nil {
					logline = us
				}
				lines = append(lines, messages.MustFromObject(&messages.Action{
					Type:    messages.Log,
					Payload: json.RawMessage(logline),
				})...)
				lines = append(lines, messages.TokenCRLF...)
			}
			return write(lines)

		case <-taskr.Done():
			bts, err := taskr.Result()
			if err != nil {
				bts = messages.GetErrActionBytes(err)
			}
			if _, err := t.writeResult(rw, bts, !(err == nil), web); err != nil {
				klog.Errorf("(h) %s failed to write result, %v", taskr.ID(), err)
			}
		}
		return false
	}
}

func (t *httpHandler) writeResult(rw http.ResponseWriter, bts []byte, isErr bool, isWeb bool) (n int, err error) {
	if isErr {
		var msg messages.Action
		err = json.Unmarshal(bts, &msg)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return rw.Write(append([]byte(err.Error()), messages.TokenCRLF...))
		}
		rw.Header().Set("Content-Type", jsonCT)
		rw.WriteHeader(http.StatusInternalServerError)
		return rw.Write(bts)
	}

	if isWeb {
		var rsp webMessage
		err = json.Unmarshal(bts, &rsp)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return rw.Write(append([]byte(err.Error()), messages.TokenCRLF...))
		}
		// bts not is web message or raw is true fallback to json message
		// https://github.com/golang/go/blob/master/src/net/http/server.go#L1098
		if rsp.Raw || rsp.StatusCode < 100 || rsp.StatusCode > 999 {
			rw.Header().Set("Content-Type", jsonCT)
			return rw.Write(bts)
		}
		return t.writeWebResult(rw, rsp)
	}

	rw.Header().Set("Content-Type", jsonCT)
	return rw.Write(bts)
}
