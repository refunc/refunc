package httptrigger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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
	if req.Method == http.MethodPost || req.Method == http.MethodDelete {
		if req.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
			err = req.ParseForm()
			if err != nil {
				return
			}
			params := make(map[string]interface{})
			for k, v := range req.Form {
				params[k] = v[0]
			}
			args, err = json.Marshal(params)
			return
		}
		args, err = ioutil.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return
		}
	} else if req.Method == http.MethodGet {
		params := req.URL.Query()
		if kwargs, has := params["_kwargs"]; has {
			// only pick the first
			args = []byte(kwargs[0])
		}
		return
	} else {
		return nil, fmt.Errorf("loader: unsupported request type %s", req.Method)
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
		return nil, err
	}

	// using golang's JSON to ensure keys are sorted
	bts, err := json.Marshal(argsmap)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bts), nil
}
