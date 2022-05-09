package logger

import (
	"context"
	"fmt"
)

type Logger interface {
	WriteLog(streamName string, bts []byte)
}

type Creator func(ctx context.Context) (Logger, error)

var loggers = make(map[string]Creator)

func Register(name string, register Creator) {
	loggers[name] = register
}

func CreateLogger(ctx context.Context, name string) (Logger, error) {
	f, ok := loggers[name]
	if ok {
		return f(ctx)
	}
	return nil, fmt.Errorf("invalid logger: %s", name)
}
