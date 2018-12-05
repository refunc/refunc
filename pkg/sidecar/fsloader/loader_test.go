package fsloader

import (
	"context"
	"flag"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/klog"
)

func init() {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	fs.Set("logtostderr", "true")
	fs.Set("v", "5")
	fs.Parse(os.Args)

	rand.Seed(time.Now().UnixNano())
}

func TestNewLoader(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		pre     func(string, *testing.T, context.CancelFunc)
	}{
		{"Exist", false, func(f string, t *testing.T, c context.CancelFunc) {
			ioutil.WriteFile(filepath.Join(f, ConfigFile), testCfgData, 0755)
		}},
		{"Wait>Load", false, func(f string, t *testing.T, c context.CancelFunc) {
			go func() {
				<-time.After(time.Duration(rand.Intn(200)) * time.Millisecond)
				ioutil.WriteFile(filepath.Join(f, ConfigFile), testCfgData, 0755)
			}()
		}},
		{"Create>Write>Load", false, func(f string, t *testing.T, c context.CancelFunc) {
			go func() {
				<-time.After(time.Duration(rand.Intn(100)) * time.Millisecond)
				file, err := os.OpenFile(filepath.Join(f, ConfigFile), os.O_RDWR|os.O_CREATE, 0755)
				if err != nil {
					t.Fatal(err)
				}
				<-time.After(time.Duration(rand.Intn(50)) * time.Millisecond)
				file.Write(testCfgData)
				<-time.After(time.Duration(rand.Intn(50)) * time.Millisecond)
				file.Sync()
				<-time.After(time.Duration(rand.Intn(10)) * time.Millisecond)
				file.Close()
			}()
		}},
		{"ContextCancel", false, func(f string, t *testing.T, c context.CancelFunc) {
			go func() {
				<-time.After(time.Duration(rand.Intn(100)) * time.Millisecond)
				c()
			}()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder, err := ioutil.TempDir("", "example")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(folder)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.pre != nil {
				tt.pre(folder, t, cancel)
			}

			got, err := NewLoader(ctx, folder)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLoader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// open & reopen
			for n := 0; n < 1; n++ {
				select {
				case <-ctx.Done():
				case <-got.C():
				}

				if ctx.Err() == context.Canceled {
					return
				}
				if fn := got.Function(); fn == nil {
					t.Error("NewLoader() failed to get function config")
					return
				}
			}
		})
	}
}

var testCfgData = []byte(`{
  "metadata":{
    "name":"updateksid",
    "namespace":"appleoperation",
    "labels":{
    "refunc.io/hash":"3f596ffd0ea5e252847a90695a1d165d",
    "refunc.io/name":"updateksid",
    "refunc.io/res":"funcinst",
    "refunc.io/trigger":"updateksid",
    "refunc.io/trigger-type":"eventgateway"
    }
  },
  "spec":{
    "hash":"3f596ffd0ea5e252847a90695a1d165d",
    "entry":"refunc /refunc-data/root/main.py",
    "maxReplicas":1,
    "runtime":{
    "name":"python36-db",
    "envs":{
      "AWS_ACCESS_KEY_ID":"IUFD156ZIEVN6MWHL12U",
      "AWS_REGION":"us-east-1",
      "AWS_SECRET_ACCESS_KEY":"7hD5eSzP_ID6dz-5o79k4BiHpKVhmSe0V-6XQ93R"
    },
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
