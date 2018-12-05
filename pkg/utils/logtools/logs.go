package logtools

import (
	"k8s.io/klog"
)

// GoLogLevel is the default v-level to write go's log out
var GoLogLevel = klog.Level(4)

// GlogWriter serves as a bridge between the standard log package and the glog package.
type GlogWriter klog.Level

// Write implements the io.Writer interface.
func (writer GlogWriter) Write(data []byte) (n int, err error) {
	klog.V(klog.Level(writer)).Info(string(data))
	return len(data), nil
}
