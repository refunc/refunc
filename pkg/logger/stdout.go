package logger

import (
	"context"
	"os"
)

type stdoutLogger struct{}

func (stdoutLogger) Name() string                           { return "stdout" }
func (stdoutLogger) WriteLog(streamName string, bts []byte) { os.Stdout.Write(bts) }

func CreateStdoutLogger(ctx context.Context, cfg string) (Logger, error) {
	return stdoutLogger{}, nil
}

func init() {
	Register("stdout", CreateStdoutLogger)
}
