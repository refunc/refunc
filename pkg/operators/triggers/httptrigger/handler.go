package httptrigger

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"k8s.io/klog"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

var (
	blockTickerCh = make(chan time.Time)
)

const (
	jsonCT = "application/json; charset=utf-8"
)

type httpHandler struct {
	fndKey string
	ns     string
	name   string
	hash   string

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

	trigger, err := t.operator.TriggerLister.Triggers(t.ns).Get(t.name)
	if err != nil {
		klog.Errorf("(h) %s trigger not found", t.fndKey)
		return
	}

	// Methods /ns/refunc-name
	sr.HandleFunc("", t.taskCreationHandler(streamingOff))
	// Methods /ns/refunc-name/*
	sr.PathPrefix("/").HandlerFunc(t.taskCreationHandler(streamingOff))

	// GET /ns/refunc-name/_meta
	// query metadata of the refunc
	sr.HandleFunc("/_meta", t.handleMeta).Methods(http.MethodGet)

	// setup http.Handler configs
	if trigger.Spec.HTTP == nil {
		if len(t.operator.corsOpts) > 0 {
			sr.Use(handlers.CORS(t.operator.corsOpts...))
		}
		return
	}

	// config cors
	var corsOpts []handlers.CORSOption
	CORS := trigger.Spec.HTTP.Cors
	if CORS.AllowCredentials {
		corsOpts = append(corsOpts, handlers.AllowCredentials())
	}
	if CORS.MaxAge > 0 {
		corsOpts = append(corsOpts, handlers.MaxAge(CORS.MaxAge))
	}
	if len(CORS.AllowOrigins) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedOrigins(CORS.AllowOrigins))
	}
	if len(CORS.AllowMethods) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedMethods(CORS.AllowMethods))
	}
	if len(CORS.AllowHeaders) > 0 {
		corsOpts = append(corsOpts, handlers.AllowedHeaders(CORS.AllowHeaders))
	}
	if len(CORS.ExposeHeaders) > 0 {
		corsOpts = append(corsOpts, handlers.ExposedHeaders(CORS.ExposeHeaders))
	}
	if len(corsOpts) > 0 {
		sr.Use(handlers.CORS(corsOpts...))
	} else if len(t.operator.corsOpts) > 0 {
		sr.Use(handlers.CORS(t.operator.corsOpts...))
	}
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

		// parse http.request to event
		event, err := formatRequestPayload(req)
		if err != nil {
			writeHTTPError(rw, http.StatusBadRequest, `request format to event fail`)
			return
		}

		// create request
		id := path.Join(t.fndKey, event.Context.RequestID)

		request := &messages.InvokeRequest{
			Args:      messages.MustFromObject(event),
			RequestID: event.Context.RequestID,
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
		t.taskPoller(ctx, rw, taskr, blockTickerCh)()
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
	taskr client.TaskResolver,
	tickerC <-chan time.Time,
) func() bool {

	write := t.flushWriter(rw, taskr.ID())

	return func() bool {
		select {
		case <-ctx.Done():
			klog.V(3).Infof("(h) %s, %v", taskr.ID(), ctx.Err())
			write(messages.GetErrActionBytes(ctx.Err()))

		case <-tickerC:
			// write ping
			return write(messages.PingMsg)

		case <-taskr.Done():
			bts, err := taskr.Result()
			if err != nil {
				bts = messages.GetErrActionBytes(err)
			}
			if _, err := t.writeResult(rw, bts, !(err == nil)); err != nil {
				klog.Errorf("(h) %s failed to write result, %v", taskr.ID(), err)
			}
		}
		return false
	}
}

func (t *httpHandler) writeResult(rw http.ResponseWriter, bts []byte, isErr bool) (n int, err error) {
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

	// https://docs.aws.amazon.com/lambda/latest/dg/urls-invocation.html#urls-payloads
	rsp, err := formatResponsePayload(bts)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return rw.Write(append([]byte(err.Error()), messages.TokenCRLF...))
	}

	body := []byte(rsp.Body)
	if rsp.IsBase64Encoded {
		body, err = base64.StdEncoding.DecodeString(rsp.Body)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return rw.Write([]byte(err.Error()))
		}
	}

	rw.Header().Set("Content-Type", mimetype.Detect(body).String())
	for k, v := range rsp.Headers {
		rw.Header().Set(k, v)
	}

	for _, cookie := range rsp.Cookies {
		rw.Header().Add("Set-Cookie", cookie)
	}

	rw.WriteHeader(rsp.StatusCode)

	return rw.Write(body)
}
