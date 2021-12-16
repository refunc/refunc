package env

import (
	"fmt"
	"net/url"

	nats "github.com/nats-io/nats.go"
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

	// connect to nats
	natsConn, err = nats.Connect(GlobalNatsURLString(), opts...)
	return
}
