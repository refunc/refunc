package env

import (
	"fmt"
	"net/url"
	"strings"

	nats "github.com/nats-io/nats.go"
	"k8s.io/klog"
)

// GlobalNatsURLString returns nats url derived from current env
func GlobalNatsURLString() (natsURL string) {
	if GlobalToken != "" {
		natsURL = (&url.URL{
			Scheme: "nats",
			User:   url.User(GlobalToken),
			Host:   GlobalNATSEndpoint,
		}).String()
	} else if GlobalAccessKey != "" && GlobalSecretKey != "" {
		natsURL = (&url.URL{
			Scheme: "nats",
			User:   url.UserPassword(GlobalAccessKey, GlobalSecretKey),
			Host:   GlobalNATSEndpoint,
		}).String()
	} else {
		natsURL = fmt.Sprintf("nats://%s", GlobalNATSEndpoint)
	}
	return
}

// NewNatsConn returns new nats connection from current env with given(optsional) options
func NewNatsConn(opts ...nats.Option) (natsConn *nats.Conn, err error) {
	if GlobalToken != "" {
		opts = append([]nats.Option{nats.Token(GlobalToken)}, opts...)
	} else if GlobalAccessKey != "" && GlobalSecretKey != "" {
		opts = append([]nats.Option{nats.UserInfo(GlobalAccessKey, GlobalSecretKey)}, opts...)
	}

	useOpts := []nats.Option{
		// never give up reconnect nats
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			klog.Errorf("nats %s disconnect, error %v", strings.Join(c.Servers(), ","), err)
		}),
		nats.ClosedHandler(func(c *nats.Conn) {
			klog.Errorf("nats %s connect closed", strings.Join(c.Servers(), ","))
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			klog.Warningf("nats %s reconnect", strings.Join(c.Servers(), ","))
		}),
	}
	useOpts = append(useOpts, opts...)

	// connect to nats
	natsConn, err = nats.Connect(GlobalNatsURLString(), useOpts...)
	return
}
