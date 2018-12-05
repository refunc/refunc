package client

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/klog"
)

func init() {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	fs.Set("logtostderr", "true")
	fs.Parse(os.Args)
}

func TestInvoke(t *testing.T) {
	dctx := func() context.Context {
		return DefaultContext
	}

	type args struct {
		ctx      context.Context
		endpoint string
		body     interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{"multifactor/tcap", args{ctx: dctx(), endpoint: "multifactor/tcap", body: map[string]interface{}{"date": "20170214"}}, nil, false},
		{"antmanler/example", args{ctx: (dctx()), endpoint: "antmanler/example", body: map[string]interface{}{"a": 1, "b": 2}}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Invoke(tt.args.ctx, tt.args.endpoint, tt.args.body)
			if (err != nil) != tt.wantErr {
				if strings.Contains(err.Error(), "404 page not found") {
					return
				}
				t.Errorf("Invoke() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			t.Logf("Invoke() len(got) = %v", len(got))
			time.Sleep(time.Millisecond)
		})
	}
}
