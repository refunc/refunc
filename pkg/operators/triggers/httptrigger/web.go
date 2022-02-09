package httptrigger

import (
	"encoding/base64"
	"net/http"
)

type webMessage struct {
	Header     map[string]string `json:"header"`
	StatusCode int               `json:"code"`
	// base64 encoded body data
	Body string `json:"body"`
}

func (t *httpHandler) writeWebResult(rw http.ResponseWriter, rsp webMessage) (n int, err error) {
	body, err := base64.StdEncoding.DecodeString(rsp.Body)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return rw.Write([]byte(err.Error()))
	}
	// write http response
	ct := http.DetectContentType(body)
	rw.Header().Set("Content-Type", ct)
	for k, v := range rsp.Header {
		rw.Header().Set(k, v)
	}
	rw.WriteHeader(rsp.StatusCode)
	return rw.Write(body)
}
