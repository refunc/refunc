package sidecar

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog"
)

var LogFrameDelimer = []byte{165, 90, 0, 1} //0xA55A0001
var LogStreamSuffix = ".log.pipe"

func (sc *Sidecar) watchLogs() {
	fis, err := os.ReadDir(RefuncRoot)
	if err != nil {
		klog.Errorf("init watch log stream error %v", err)
	}
	for _, fi := range fis {
		if !strings.HasSuffix(fi.Name(), LogStreamSuffix) {
			continue
		}
		wid := strings.TrimSuffix(fi.Name(), LogStreamSuffix)
		fd, err := os.OpenFile(filepath.Join(RefuncRoot, fi.Name()), os.O_RDWR, fs.ModeNamedPipe)
		if err != nil {
			klog.Errorf("open log stream error %s %v", wid, err)
			continue
		}
		sc.logStreams.Store(wid, fd)
		go sc.tailLog(wid, fd)
	}
WATCH_LOOP:
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Errorf("failed to create logs watcher, %v", err)
		return
	}
	defer watcher.Close()
	err = watcher.Add(RefuncRoot)
	if err != nil {
		klog.Errorf("failed to watch logs, %v", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				goto WATCH_LOOP
			}
			if (event.Op == fsnotify.Create || event.Op == fsnotify.Write) && strings.HasSuffix(event.Name, LogStreamSuffix) {
				wid := strings.TrimPrefix(strings.TrimSuffix(event.Name, LogStreamSuffix), fmt.Sprintf("%s/", RefuncRoot))
				if _, ok := sc.logStreams.Load(wid); ok {
					continue
				}
				fd, err := os.OpenFile(event.Name, os.O_RDWR, fs.ModeNamedPipe)
				if err != nil {
					klog.Errorf("open log stream error %s %v", wid, err)
					continue
				}
				sc.logStreams.Store(wid, fd)
				go sc.tailLog(wid, fd)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				goto WATCH_LOOP
			}
			klog.Errorf("logs watcher error %v", err)
		}
	}
}

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
