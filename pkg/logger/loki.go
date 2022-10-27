package logger

import (
	"context"
	"strings"
	"time"

	"github.com/Arvintian/loki-client-go/loki"
)

type lokiLogger struct {
	url string
	c   *loki.Client
}

func (l lokiLogger) Name() string { return "loki" }

func (l lokiLogger) WriteLog(streamName string, bts []byte) {
	// refunc.<namespace>.<func-name>.logs.<funcinsts-id>.<worker-id>
	streamInfo := strings.Split(streamName, ".")
	l.c.Handle(map[string]string{
		"namespace": streamInfo[1],
		"funcdef":   streamInfo[2],
		"funcinsts": streamInfo[4],
	}, time.Now(), string(bts))
}

func CreateLokiLogger(ctx context.Context, cfg string) (Logger, error) {
	lokiClient, err := loki.NewWithDefault(cfg)
	if err != nil {
		return nil, err
	}
	return lokiLogger{
		url: cfg,
		c:   lokiClient,
	}, nil
}

func init() {
	Register("loki", CreateLokiLogger)
}
