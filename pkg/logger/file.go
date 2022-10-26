package logger

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"k8s.io/klog"
)

type fileLogger struct {
	fd   *os.File
	mu   *sync.Mutex
	path string
}

func (l fileLogger) Name() string { return "file" }

func (l fileLogger) WriteLog(streamName string, bts []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fd == nil {
		fileFd, err := openLogFile(l.path, streamName)
		if err != nil {
			klog.Error(err)
			return
		}
		l.fd = fileFd
	}
	l.fd.Write(bts)
}

func openLogFile(path, streamName string) (*os.File, error) {
	// refunc.<namespace>.<func-name>.logs.<funcinsts-id>.<worker-id>
	streamInfo := strings.Split(streamName, ".")
	fileDir := fmt.Sprintf("%s/%s/%s", path, streamInfo[1], streamInfo[2])
	filePath := fmt.Sprintf("%s/%s", fileDir, streamInfo[4])
	if _, err := os.Stat(fileDir); os.IsNotExist(err) {
		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return nil, err
		}
	}
	fileFd, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return fileFd, nil
}

func CreateFileLogger(ctx context.Context, cfg string) (Logger, error) {
	testFile, err := ioutil.TempFile(cfg, "logger-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(testFile.Name())
	return fileLogger{
		mu:   &sync.Mutex{},
		path: cfg,
	}, nil
}

func init() {
	Register("file", CreateFileLogger)
}
