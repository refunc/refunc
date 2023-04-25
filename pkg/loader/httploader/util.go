package httploader

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"k8s.io/klog"
)

func checkServerErr(err error) error {
	if err != nil && (strings.Contains(err.Error(), "use of closed network connection") || strings.Contains(err.Error(), "Server closed")) {
		return nil
	}
	return err
}

type logWriter struct {
	sync.RWMutex
	current io.Writer
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	lw.RLock()
	n, err = lw.current.Write(p)
	lw.RUnlock()
	return
}

func (lw *logWriter) Switch(w io.Writer) {
	lw.Lock()
	defer lw.Unlock()
	if w == nil {
		lw.current = ioutil.Discard
	} else {
		lw.current = w
	}
}

var klogWriter = new(logWriter)

func init() {
	klogWriter.current = ioutil.Discard
	klog.SetOutputBySeverity("INFO", klogWriter)
}

type noOpRspWriter struct{}

func (noOpRspWriter) Header() http.Header       { return make(http.Header) }
func (noOpRspWriter) Write([]byte) (int, error) { return 0, nil }
func (noOpRspWriter) WriteHeader(int)           {}
