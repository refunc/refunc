package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	nats "github.com/nats-io/go-nats"
	"github.com/refunc/refunc/pkg/messages"
)

// DefaultContext for current env
var DefaultContext context.Context

var (
	loggerKey struct {
		_1 struct{}
	}
	execEnvKey struct {
		_2 struct{}
	}
	httpCliKey struct {
		_3 struct{}
	}
	rawMsgKey struct {
		_4 struct{}
	}
	// store paresed envmap in context
	envMapKey struct {
		_5 struct{}
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

// Name returns the name of current environment
func Name(ctx context.Context) string {
	return getEnv(ctx).Name
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
// this is different for context.WithTimeout, this is set a hint that pass to remote
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
	}
	if hint, ok := ctx.Value(timeoutKey).(time.Duration); ok && hint > 0 {
		if hint < timeout {
			return hint
		}
	}
	return timeout
}

// GetEnviron returns envrionment variables of current context
func GetEnviron(ctx context.Context) []string {
	return getEnv(ctx).Environ
}

// GetRawReqeustMessage retrieves *messages.RequestedMsg that assocated within the given context
func GetRawReqeustMessage(ctx context.Context) *messages.InvokeRequest {
	if v := ctx.Value(rawMsgKey); v != nil {
		if req, ok := v.(*messages.InvokeRequest); ok {
			return req
		}
	}
	return nil
}

// WithParsedEnv parse environ to a map[string]string
func WithParsedEnv(ctx context.Context) context.Context {
	if val := ctx.Value(envMapKey); val == nil {
		return context.WithValue(ctx, envMapKey, envMap(GetEnviron(ctx)))
	}
	return ctx
}

// GetEnvironMap returns a parsed environments map
func GetEnvironMap(ctx context.Context) map[string]string {
	if val := ctx.Value(envMapKey); val != nil {
		return val.(map[string]string)
	}
	return envMap(GetEnviron(ctx))
}

func envMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ)+1)
	for i := range environ {
		parts := strings.SplitN(environ[i], "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		env[strings.ToUpper(parts[0])] = parts[1]
	}
	return env
}

func setRequestedMessage(parent context.Context, req *messages.InvokeRequest) context.Context {
	return context.WithValue(parent, rawMsgKey, req)
}

func withRootEnv(parent context.Context) context.Context {
	return withEnv(parent, rootEnv)
}

func withEnv(parent context.Context, env *execEnv) context.Context {
	return context.WithValue(parent, execEnvKey, env)
}

func getEnv(ctx context.Context) *execEnv {
	if v := ctx.Value(execEnvKey); v != nil {
		return v.(*execEnv)
	}
	return rootEnv.Copy()
}

type nopLogger struct{}

var emptyLogger = new(nopLogger)

func (*nopLogger) Infof(string, ...interface{}) {}
func (*nopLogger) Info(...interface{})          {}
