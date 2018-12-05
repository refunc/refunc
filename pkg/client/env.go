package client

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
)

// execEnv describes current invocation environment
type execEnv struct {
	BaseURL string `json:"baseURL"`
	Name    string `json:"name"`

	InCluster bool

	Environ []string
}

// root env construct at loading time
var rootEnv *execEnv

func (ee *execEnv) Copy() *execEnv {
	ne := *ee
	return &ne
}

// newEnv returns a copy of root
func newEnv() *execEnv {
	return rootEnv.Copy()
}

// load env
func init() {
	rootEnv = new(execEnv)

	// set defaults
	rootEnv.BaseURL = environForKey("REFUNC_ROUTER_BASE", "")

	// get namespace and name from environments
	ns := environForKey("REFUNC_NAMESPACE", "")
	name := environForKey("REFUNC_NAME", "")

	cfgPath := environForKey("REFUNC_CONFIG", filepath.Join(homeDir(), ".v87/config.yaml"))
	rootEnv.InCluster = environForKey("REFUNC_ENV", "") == "cluster"

	switch {
	case rootEnv.InCluster: // in cluster
		if rootEnv.BaseURL == "" {
			// service in k8s
			rootEnv.BaseURL = "http://gateway.refunc.svc.cluster.local:7788"
		}
		name = ns + "/" + name
	case os.Getenv("JPY_USER") != "": // in jupyter kernel
		// currently v87hub & refunc in the same k8s cluster
		if rootEnv.BaseURL == "" {
			// service in k8s
			rootEnv.BaseURL = "http://gateway.refunc.svc.cluster.local:7788"
		}
		name = os.Getenv("JPY_USER") + "/jpkernel"
	case os.Getenv("REFUNC_APP") != "": // in app mode
		name = os.Getenv("REFUNC_APP") + "/apps"
	case fileExists(cfgPath): // local
		var cfg struct {
			Username string `json:"username"`
			Refunc   struct {
				BaseURL   string `json:"baseURL"`
				RecvLog   bool   `json:"recvLog"`
				CacheBase string `json:"cacheBase"`
			} `json:"refunc"`
		}
		bts, err := ioutil.ReadFile(cfgPath)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(bts, &cfg)
		if err != nil {
			panic(err)
		}
		rootEnv.BaseURL = cfg.Refunc.BaseURL
		name = cfg.Username + "/local"

	default:
		panic("client: cannot decide current env")
	}

	if rootEnv.BaseURL == "" {
		// set default URL outof cluster
		rootEnv.BaseURL = "https://gw.refunc.io"
	}
	rootEnv.BaseURL = strings.TrimRight(rootEnv.BaseURL, "/")
	rootEnv.Name = name

	// setup default context
	DefaultContext = withRootEnv(context.TODO())
}
