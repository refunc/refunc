package client

import (
	observer "github.com/refunc/go-observer"
)

// TaskResolver is interface of background running task
type TaskResolver interface {
	// ID unique string to identify this task
	ID() string

	// Name returns a human readable string
	Name() string

	Done() <-chan struct{}
	Cancel()

	// Result returns the ouput of task once exited
	Result() ([]byte, error)

	MsgObserver() observer.Stream

	LogObserver() observer.Stream

	// StatJSON returns statistic info of running task
	StatJSON() string
}

// Logger is interface that canbe accessed from context
type Logger interface {
	Infof(format string, args ...interface{})
	Info(args ...interface{})
}
