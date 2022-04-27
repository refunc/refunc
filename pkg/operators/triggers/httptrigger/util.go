package httptrigger

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

func writeHTTPError(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", jsonCT)
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.WriteHeader(code)
	fmt.Fprintf(rw, "{\"code\":%d,\"msg\":%q}\r\n", code, msg)
}

func flushRW(rw http.ResponseWriter) {
	if f, ok := rw.(interface {
		Flush()
	}); ok {
		f.Flush()
	}
}

// GetPayload retreive args' payload from request object
func GetPayload(req *http.Request) (args []byte, err error) {
	args, err = ioutil.ReadAll(req.Body)
	req.Body.Close()
	req.Body = ioutil.NopCloser(bytes.NewReader(args))
	return
}

type Request struct {
	Method        string      `json:"method"`
	Header        http.Header `json:"header"`
	Body          string      `json:"body"`
	ContentLength int64       `json:"contentLength"`
	Host          string      `json:"host"`
	Form          url.Values  `json:"form"`
	PostForm      url.Values  `json:"postForm"`
	RemoteAddr    string      `json:"remoteAddr"`
	RequestURI    string      `json:"requestURI"`
}

func GetRequest(httpReq *http.Request, body []byte) (req Request, err error) {
	err = httpReq.ParseForm()
	if err != nil {
		return
	}
	req = Request{
		Method:        httpReq.Method,
		Header:        httpReq.Header,
		ContentLength: httpReq.ContentLength,
		Body:          base64.StdEncoding.EncodeToString(body),
		Host:          httpReq.Host,
		Form:          httpReq.Form,
		PostForm:      httpReq.PostForm,
		RemoteAddr:    httpReq.RemoteAddr,
		RequestURI:    httpReq.RequestURI,
	}
	return
}

// SortArgs parses json args, and sorted by key
func SortArgs(args []byte) (json.RawMessage, error) {
	if len(args) == 0 {
		// empty input
		return json.RawMessage([]byte{'{', '}'}), nil
	}
	var argsmap map[string]interface{}
	if err := json.Unmarshal(args, &argsmap); err != nil {
		// unable parse json args, fallback to empty input
		return json.RawMessage([]byte{'{', '}'}), nil
	}

	// using golang's JSON to ensure keys are sorted
	bts, err := json.Marshal(argsmap)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bts), nil
}

func GetRequestID(req *http.Request) string {
	id := req.Header.Get("X-Request-ID")
	if id == "" {
		id = uuid.New().String()
	}
	return id
}
