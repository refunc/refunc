package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

var ErrBaseURLIsEmpty = errors.New("http_resovler: Base URL is not set")

// NewHTTPResolver creates a new task and starts a resolver at `endpoint`
func NewHTTPResolver(ctx context.Context, endpoint string, request *messages.InvokeRequest) (TaskResolver, error) {
	// clean path
	endpoint = strings.Trim(endpoint, "/")
	params := url.Values{}
	if IsLoggingEnabled(ctx) {
		// enable logs pulling
		params.Add("recv_log", "true")
	}
	baseURL := GetHTTPBaseURL(ctx)
	if baseURL == "" {
		return nil, ErrBaseURLIsEmpty
	}
	reqURL := baseURL + "/" + endpoint + "/tasks" + "?" + params.Encode()

	logger := GetLogger(ctx)

	SetReqeustDeadline(ctx, request)
	ctx = WithTimeoutHint(ctx, request.Deadline.Sub(time.Now()))

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	id := utils.GenID([]byte(endpoint), body)
	name := fmt.Sprintf("%s<r%s>", endpoint, id[7:14])
	tr := NewSimpleResolver(id, name)

	tr.InputStream = newTaskReader(ctx, func(ctx context.Context) (io.ReadCloser, error) {
		cli := getClient(ctx)
		logger.Infof("request to %s", reqURL)

		req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		// set headers
		req.Header.Set("User-Agent", "go-refunc v1")
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("X-Refunc-User", strings.TrimSuffix(Name(ctx), "/local"))

		rsp, err := cli.Do(req)
		if err != nil {
			return nil, err
		}
		return rsp.Body, nil
	})

	if IsLoggingEnabled(ctx) {
		ls := tr.LogObserver()
		go func() {
			for {
				select {
				case <-ls.Changes():
				case <-ctx.Done():
					return
				}
				logger.Infof("[%s] %v", endpoint, ls.Next())
			}
		}()
	}

	return tr.Start(ctx), nil
}

var defaultClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 15 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 100 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

func getClient(ctx context.Context) *http.Client {
	if v := ctx.Value(httpCliKey); v != nil {
		return v.(*http.Client)
	}
	return defaultClient
}

type taskReader struct {
	ctx    context.Context
	cancel context.CancelFunc

	getReader func(ctx context.Context) (io.ReadCloser, error)

	buf  *bytes.Buffer
	bufC chan []byte

	io struct {
		sync.Once

		closeC chan struct{}
		closed bool

		err error
	}
}

func newTaskReader(ctx context.Context, readerFn func(ctx context.Context) (io.ReadCloser, error)) *taskReader {
	const (
		maxTimeout = 4 * time.Hour
		chunk4k    = 4 * 2 << 10 // 512KB
	)
	var bufZone [chunk4k]byte

	tr := &taskReader{
		getReader: readerFn,
		buf:       bytes.NewBuffer(bufZone[:0]),
		bufC:      make(chan []byte),
	}
	tr.ctx, tr.cancel = context.WithTimeout(ctx, maxTimeout)
	tr.io.closeC = make(chan struct{})

	go tr.ioloop()
	return tr
}

func (tr *taskReader) Read(p []byte) (n int, err error) {
	if tr == nil {
		return 0, errors.New("taskreader: invalid reader")
	}

	if len(p) == 0 {
		return 0, nil
	}

	for {
		nr, err := tr.buf.Read(p)
		n, p = n+nr, p[nr:]
		if len(p) == 0 {
			return n, err
		}

		if tr.io.closed && err == io.EOF {
			return n, tr.io.err
		}

		// block and wait more data
		for {
			bts, ok := <-tr.bufC
			if !ok {
				break
			}
			nw, err := tr.buf.Write(bts)
			if err != nil {
				return n, err
			}
			if nw != len(bts) {
				return n, errors.New("taskReader: short wirte")
			}
			if tr.buf.Len() >= len(p) {
				break
			}
		}
	}
}

func (tr *taskReader) Close() error {
	tr.io.Do(func() {
		tr.cancel()
		// wait for ioloop
		<-tr.io.closeC
		tr.io.closed = true
	})
	return tr.io.err
}

func (tr *taskReader) ioloop() {
	defer tr.Close()
	defer close(tr.io.closeC)
	defer close(tr.bufC)
	defer tr.cancel() // avoid context leak

	// get logger
	logger := GetLogger(tr.ctx)

	defer func() {
		if r := recover(); r != nil {
			utils.LogTraceback(r, 4, logger)
			tr.io.err = fmt.Errorf("panic(%v)", r)
		}
	}()

	const (
		maxRetries = 17
		maxDelay   = 3 * time.Second
	)
	var (
		retryCnt int
		delayDur = 2 * time.Millisecond
	)

	// start polling response with retry, until EOF or context been canceled
	for !tr.io.closed && retryCnt < maxRetries && func() bool {
		reqCtx, cancel := context.WithCancel(tr.ctx)
		defer cancel()

		rc, err := tr.getReader(reqCtx)
		if err != nil {
			// requesting failed, delay
			select {
			case <-time.After(delayDur): // have a rest
			case <-reqCtx.Done():
				return false
			}
			// exp backoff retry
			if 2*delayDur > maxDelay {
				delayDur = maxDelay
			} else {
				delayDur *= 2
			}
			return true
		}

		var conce sync.Once
		closeRC := func() { conce.Do(func() { rc.Close() }) }

		// ensure we close the body
		defer closeRC()

		errC := make(chan error)
		defer close(errC)

		// bumper will exit on rsp.Body close
		go func() {
			scanner := utils.NewScanner(rc)
			for scanner.Scan() {
				// take a copy of bytes, to avoid buffer overlap
				length := len(scanner.Bytes())
				bts := make([]byte, length, length+len(messages.TokenCRLF))
				copy(bts, scanner.Bytes())
				select {
				case tr.bufC <- append(bts, messages.TokenCRLF...):
				case <-reqCtx.Done():
					errC <- reqCtx.Err() // funcinst retrying
					return
				}
			}

			if scanner.Err() != nil {
				errC <- scanner.Err() // funcinst retrying
			} else {
				errC <- io.EOF // finished
			}
		}()

		select {
		case <-reqCtx.Done():
			closeRC()
			select {
			case tr.io.err = <-errC:
			case <-time.After(maxDelay):
				tr.io.err = errors.New("closing bumper timeout")
			}
			return false

		case err := <-errC:
			if err != io.EOF {
				logger.Infof("retry on error: %v", err)
				// try again
				return true
			}
			tr.io.err = io.EOF
			return false
		}
	}() {
		retryCnt++
	}
}
