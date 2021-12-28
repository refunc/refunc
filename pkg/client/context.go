package client

import (
	"context"
	"net/http"
	"time"

	nats "github.com/nats-io/nats.go"
)

// DefaultContext for current env
var DefaultContext context.Context

var (
	nameKey struct {
		_0 struct{}
	}
	loggerKey struct {
		_1 struct{}
	}
	// execEnvKey struct {
	// 	_2 struct{}
	// }
	httpCliKey struct {
		_3 struct{}
	}
	httpBaseURLKey struct {
		_4 struct{}
	}
	natsKey struct {
		_6 struct{}
	}
	logKey struct {
		_7 struct{}
	}
	timeoutKey struct {
		_8 struct{}
	}
)

func WithName(parent context.Context, name string) context.Context {
	return context.WithValue(parent, nameKey, name)
}

// Name returns the name of current environment
func Name(ctx context.Context) string {
	if v := ctx.Value(nameKey); v != nil {
		return v.(string)
	}
	return "dev"
}

// WithLogger set new logger to current context
func WithLogger(parent context.Context, logger Logger) context.Context {
	return context.WithValue(parent, loggerKey, logger)
}

// GetLogger retrieves logger that assocated within the given context
func GetLogger(ctx context.Context) Logger {
	if v := ctx.Value(loggerKey); v != nil {
		return v.(Logger)
	}
	return emptyLogger
}

// WithHTTPClient sets a custom configured http.Client
func WithHTTPClient(parent context.Context, client *http.Client) context.Context {
	if client != nil {
		return context.WithValue(parent, httpCliKey, client)
	}
	return parent
}

// WithHTTPBaseURL sets a custom configured http.Client
func WithHTTPBaseURL(parent context.Context, baseURL string) context.Context {
	return context.WithValue(parent, httpBaseURLKey, baseURL)
}

// GetHTTPBaseURL returns http BaseURL associated current context
func GetHTTPBaseURL(ctx context.Context) string {
	if v := ctx.Value(httpBaseURLKey); v != nil {
		return v.(string)
	}
	return ""
}

// WithNatsConn sets a valid nats connetion to use NATS based RPC
func WithNatsConn(parent context.Context, conn *nats.Conn) context.Context {
	if conn != nil {
		return context.WithValue(parent, natsKey, conn)
	}
	return parent
}

// WithLoggingHint sets log forwarding hint
func WithLoggingHint(parent context.Context, enabled bool) context.Context {
	return context.WithValue(parent, logKey, enabled)
}

// IsLoggingEnabled checks if current context set log forwarding hint
func IsLoggingEnabled(ctx context.Context) bool {
	if val := ctx.Value(logKey); val != nil {
		return val.(bool)
	}
	return false
}

// WithTimeoutHint sets timeout for invocation,
// this is different for context.WithTimeout, this is a hint that will be past to remote
func WithTimeoutHint(parent context.Context, timeout time.Duration) context.Context {
	if deadline, ok := parent.Deadline(); ok && deadline.Before(time.Now().Add(timeout)) {
		timeout = deadline.Sub(time.Now())
	}
	if timeout < 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	return context.WithValue(parent, timeoutKey, timeout)
}

// GetTimeoutHint returns default timeout hint from context
func GetTimeoutHint(ctx context.Context) time.Duration {
	var timeout time.Duration // default is 0
	if deadline, ok := ctx.Deadline(); ok {
		timeout = deadline.Sub(time.Now())
		if timeout < 0 {
			timeout = 0
		}
		if hint, ok := ctx.Value(timeoutKey).(time.Duration); ok && hint > 0 {
			if hint < timeout {
				return hint
			}
		}
	} else {
		if hint, ok := ctx.Value(timeoutKey).(time.Duration); ok && hint > 0 {
			return hint
		}
	}
	return timeout
}

// func withRootEnv(parent context.Context) context.Context {
// 	return withEnv(parent, rootEnv)
// }

// func withEnv(parent context.Context, env *execEnv) context.Context {
// 	return context.WithValue(parent, execEnvKey, env)
// }

// func getEnv(ctx context.Context) *execEnv {
// 	if v := ctx.Value(execEnvKey); v != nil {
// 		return v.(*execEnv)
// 	}
// 	return rootEnv.Copy()
// }

type nopLogger struct{}

var emptyLogger = new(nopLogger)

func (*nopLogger) Infof(string, ...interface{}) {}
func (*nopLogger) Info(...interface{})          {}
