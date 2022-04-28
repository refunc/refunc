package httptrigger

import (
	"fmt"
	"net/http"

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

func GetRequestID(req *http.Request) string {
	id := req.Header.Get("X-Request-ID")
	if id == "" {
		id = uuid.New().String()
	}
	return id
}
