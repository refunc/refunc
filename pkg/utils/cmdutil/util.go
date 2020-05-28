package cmdutil

import (
	"os"
	"os/signal"
	"syscall"
)

// GetSysSig register exit signals
func GetSysSig() <-chan os.Signal {
	// handle system signal
	ch := make(chan os.Signal, 1)
	signal.Notify(
		ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGTERM,
	)
	return ch
}
