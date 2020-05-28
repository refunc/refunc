package utils

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"
	"strings"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/messages"
)

// ReallyCrash For testing, bypass HandleCrash.
var ReallyCrash bool

// HandleCrash simply catches a crash and logs an error. Meant to be called via defer.
func HandleCrash() {
	if ReallyCrash {
		return
	}

	r := recover()
	if r != nil {
		callers := ""
		for i := 1; true; i++ {
			_, file, line, ok := runtime.Caller(i)
			if !ok {
				break
			}
			callers = callers + fmt.Sprintf("%v:%v\n", file, line)
		}
		klog.Warningf("Recovered from panic: %#v (%v)\n%v", r, r, callers)
	}
}

// LogTraceback prints traceback to given logger
func LogTraceback(r interface{}, depth int, logger interface {
	Infof(fmt string, arg ...interface{})
}) {
	var callers []string
	for i := depth; true; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		callers = append(callers, fmt.Sprintf("%v:%v", file, line))
	}
	callers = callers[0 : len(callers)-1]

	logger.Infof("panic: %#v", r)
	for i := range callers {
		logger.Infof("tb| %s", callers[i])
	}
}

const (
	_byte     = 1.0
	_kilobyte = 1024 * _byte
	_megabyte = 1024 * _kilobyte
	_gigabyte = 1024 * _megabyte
	_terabyte = 1024 * _gigabyte
)

// ByteSize returns a human readable byte string, of the format 10M, 12.5K, etc.  The following units are available:
//	T Terabyte
//	G Gigabyte
//	M Megabyte
//	K Kilobyte
// the unit that would result in printing the smallest whole number is always chosen
func ByteSize(bytes uint64) string {
	unit := ""
	value := float32(bytes)

	switch {
	case bytes >= _terabyte:
		unit = "T"
		value = value / _terabyte
	case bytes >= _gigabyte:
		unit = "G"
		value = value / _gigabyte
	case bytes >= _megabyte:
		unit = "M"
		value = value / _megabyte
	case bytes >= _kilobyte:
		unit = "K"
		value = value / _kilobyte
	case bytes >= _byte:
		unit = "B"
	case bytes == 0:
		return "0"
	}

	stringValue := fmt.Sprintf("%.1f", value)
	stringValue = strings.TrimSuffix(stringValue, ".0")
	return fmt.Sprintf("%s%s", stringValue, unit)
}

// NewScanner returns a command line scanner
func NewScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	raw := [4 * 1024]byte{}
	scanner.Buffer(raw[:], messages.MaxPayloadSize)
	return scanner
}

// GenID generates an ID for given paylaod
func GenID(btss ...[]byte) string {
	hasher := md5.New()
	for _, bts := range btss {
		hasher.Write(bts)
	}
	return hex.EncodeToString(hasher.Sum(nil))[:32]
}
