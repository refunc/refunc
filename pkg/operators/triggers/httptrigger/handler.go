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
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
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
	pingInterval = 500 * time.Millisecond

	// const strings for reuse
	trueStr = "true"
	jsonCT  = "application/json; charset=utf-8"
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

		args, err := GetPayload(req)
		if err != nil {
			writeHTTPError(rw, http.StatusBadRequest, err.Error())
			return
		}

		// parse payload
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
		argsMap["$method"] = strings.ToLower(req.Method)
		data = messages.MustFromObject(argsMap)

		rid := utils.GenID(data)
		id := path.Join(t.fndKey, rid)

		// create request
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
		t.taskPoller(ctx, rw, "" /*contentType*/, taskr, blockTickerCh, false, false)()
		return
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
	rw.Write(append([]byte{'{', '}'}, messages.TokenCRLF...))
	return
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
	ct string,
	taskr client.TaskResolver,
	tickerC <-chan time.Time,
	fwdlog, isstream bool,
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
			if _, err := t.writeResult(rw, bts, isstream, ct); err != nil {
				klog.Errorf("(h) %s failed to write result, %v", taskr.ID(), err)
			}
		}
		return false
	}
}

func (t *httpHandler) writeResult(rw http.ResponseWriter, bts []byte, isstream bool, ct string) (n int, err error) {
	if !isstream {
		var msg messages.Action
		err = json.Unmarshal(bts, &msg)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return rw.Write(append([]byte(err.Error()), messages.TokenCRLF...))
		}
		if ct == "" {
			ct = jsonCT
		}
		bts = msg.Payload
	}
	if ct == "" {
		ct = http.DetectContentType(bts)
	}
	rw.Header().Set("Content-Type", ct)
	return rw.Write(bts)
}

func getContentType(trigger *rfv1beta3.Trigger) string {
	if trigger.Spec.HTTPTrigger != nil && trigger.Spec.HTTPTrigger.ContentType != "" {
		return trigger.Spec.HTTPTrigger.ContentType
	}
	return jsonCT
}
