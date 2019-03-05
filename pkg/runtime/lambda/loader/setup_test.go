package loader

import (
	"context"
	"encoding/json"
	"flag"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/klog"

	"github.com/refunc/refunc/pkg/runtime/types"
)

//nolint:errcheck
func init() {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	fs.Set("logtostderr", "true")
	fs.Set("v", "5")
	fs.Parse(os.Args)

	rand.Seed(time.Now().UnixNano())
}

func Test_simpleLoader_setup(t *testing.T) {

	var fn *types.Function
	err := json.Unmarshal(testCfgData, &fn)
	if err != nil {
		t.Fatal(err)
	}

	test := func(name string, wantErr bool, fn *types.Function) {
		t.Run(name, func(t *testing.T) {
			//nolint:errcheck
			withTmpFloder(func(base string) {
				p := func(f string) string {
					return filepath.Join(base, f)
				}

				RefuncRoot = p("/var/run/refunc")
				os.MkdirAll(RefuncRoot, 0755)
				DefaultTaskRoot = p("/var/task")
				os.MkdirAll(DefaultTaskRoot, 0755)
				DefaultRuntimeRoot = p("/var/runtime")
				os.MkdirAll(DefaultRuntimeRoot, 0755)

				DefaultMain = p("/var/runtime/bootstrap")
				AlterMainPath = p("/opt/bootstrap")

				ld := &simpleLoader{
					ctx: context.Background(),
				}
				if err := ld.setup(fn); err != nil {
					if wantErr {
						return
					}
					t.Errorf("simpleLoader.setup() error = %v", err)
				}

				if !fileExists(p("/var/run/refunc/refunc.json")) {
					t.Errorf("missing: /var/run/refunc/refunc.json")
				}

				if !fileExists(p("/var/run/refunc/.setup")) {
					t.Errorf("missing: /var/run/refunc/.setup")
				}

				if !fileExists(p("/var/task/bootstrap")) {
					t.Errorf("missing: /var/task/bootstrap")
				}
			})
		})
	}

	test("Base64", false, fn)

	//nolint:errcheck
	withTmpFloder(func(base string) {
		// start file server
		saveBase64(fn.Spec.Body, base)
		server := &http.Server{Addr: "127.0.0.1:38080", Handler: http.FileServer(http.Dir(base))}
		defer server.Shutdown(nil)
		go func() {
			server.ListenAndServe()
			t.Log("fileserver exited")
		}()

		fn.Spec.Body = "http://127.0.0.1:38080/wrong-path/lambda.zip"
		test("URL>WrongPath", true, fn)

		fn.Spec.Body = "http://127.0.0.1:38080/lambda.zip"
		test("URL", false, fn)
	})

}

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

var testCfgData = []byte(`{
  "metadata":{
    "name":"updateksid",
    "namespace":"appleoperation"
  },
  "spec":{
    "body":"base64://lambda.zip/UEsDBAoAAAAAAM2rkE0+3vEJGgAAABoAAAAJABwAYm9vdHN0cmFwVVQJAANxUxZccVMWXHV4CwABBPUBAAAEAAAAACMhL2Jpbi9iYXNoCmVjaG8gImhlbGxvIgoKUEsBAh4DCgAAAAAAzauQTT7e8QkaAAAAGgAAAAkAGAAAAAAAAQAAAOSBAAAAAGJvb3RzdHJhcFVUBQADcVMWXHV4CwABBPUBAAAEAAAAAFBLBQYAAAAAAQABAE8AAABdAAAAAAA=",
    "hash":"3f596ffd0ea5e252847a90695a1d165d",
    "entry":"/refunc-data/root/main.py",
    "maxReplicas":1,
    "runtime":{
      "name":"python36-db",
      "timeout":9,
      "credentials":{
        "accessKey":"IUFD156ZIEVN6MWHL12U",
        "secretKey":"7hD5eSzP_ID6dz-5o79k4BiHpKVhmSe0V-6XQ93R",
        "token":"40uPfxrMBFVEmUVbg"
      },
      "permissions":{
        "scope":"funcs/appleoperation/updateksid/data/",
        "publish":[
          "refunc.*.*"
        ],
        "subscribe":[
          "_INBOX.*"
        ]
      }
    }
  }
}`)
