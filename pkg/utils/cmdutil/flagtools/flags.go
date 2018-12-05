package flagtools

import (
	"flag"
	"os"
	"reflect"
	"strings"

	"github.com/refunc/refunc/pkg/utils/cmdutil/pflagenv"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

// flagValueWrapper implements pflag.Value around a flag.Value.  The main
// difference here is the addition of the Type method that returns a string
// name of the type.  As this is generally unknown, we approximate that with
// reflection.
type flagValueWrapper struct {
	inner    flag.Value
	flagType string
}

func wrapFlagValue(v flag.Value) pflag.Value {
	// If the flag.Value happens to also be a pflag.Value, just use it directly.
	if pv, ok := v.(pflag.Value); ok {
		return pv
	}

	pv := &flagValueWrapper{
		inner: v,
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Interface || t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pv.flagType = t.Name()
	pv.flagType = strings.TrimSuffix(pv.flagType, "Value")
	return pv
}

func (v *flagValueWrapper) String() string {
	return v.inner.String()
}

func (v *flagValueWrapper) Set(s string) error {
	return v.inner.Set(s)
}

func (v *flagValueWrapper) Type() string {
	return v.flagType
}

type boolFlag interface {
	flag.Value
	IsBoolFlag() bool
}

func (v *flagValueWrapper) IsBoolFlag() bool {
	if bv, ok := v.inner.(boolFlag); ok {
		return bv.IsBoolFlag()
	}
	return false
}

// Imports a 'flag.Flag' into a 'pflag.FlagSet'.  The "short" option is unset
// and the type is inferred using reflection.
func addFlagToPFlagSet(f *flag.Flag, fs *pflag.FlagSet) {
	if fs.Lookup(f.Name) == nil {
		fs.Var(wrapFlagValue(f.Value), f.Name, f.Usage)
	}
}

// AddFlagSetToPFlagSet adds all of the flags in a 'flag.FlagSet' package flags to a 'pflag.FlagSet'.
func AddFlagSetToPFlagSet(fsIn *flag.FlagSet, fsOut *pflag.FlagSet) {
	fsIn.VisitAll(func(f *flag.Flag) {
		addFlagToPFlagSet(f, fsOut)
	})
}

// AddAllFlagsToPFlags adds all of the top level 'flag' package flags to the top level 'pflag' flags.
func AddAllFlagsToPFlags() {
	AddFlagSetToPFlagSet(flag.CommandLine, pflag.CommandLine)
}

// AddPFlagSetToPFlagSet merges the flags of fsFrom into fsTo.
func AddPFlagSetToPFlagSet(fsFrom *pflag.FlagSet, fsTo *pflag.FlagSet) {
	if fsFrom != nil && fsTo != nil {
		fsFrom.VisitAll(func(f *pflag.Flag) {
			if fsTo.Lookup(f.Name) == nil {
				fsTo.AddFlag(f)
			}
		})
	}
}

// WordSepNormalizeFunc changes all flags that contain "_" separators
func WordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}

// InitFlags normalizes and parses the command line flags
func InitFlags() {
	pflag.CommandLine.SetNormalizeFunc(WordSepNormalizeFunc)
	// AddAllFlagsToPFlags()

	// hack for glog
	flag.Set("logtostderr", "true")
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	os.Args = []string{os.Args[0]}
	flag.Parse()
	os.Args = oldArgs

	flagset := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(flagset)
	// set default flags, enable klog.Set*Ouput to capture logs
	flagset.Set("alsologtostderr", "true")
	flagset.Set("log_file", "/dev/null")
	AddFlagSetToPFlagSet(flagset, pflag.CommandLine)

	// set flags from environment variables first
	pflagenv.Parse()

	// parse from command line
	pflag.Parse()
}
