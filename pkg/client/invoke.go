package client

import (
	"context"
	"strings"

	nats "github.com/nats-io/go-nats"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

// Invoke creates a task from endpoint using given `args` and waits until task finish
func Invoke(ctx context.Context, endpoint string, body interface{}) ([]byte, error) {
	var (
		taskR TaskResolver
		err   error
	)
	req := &messages.InvokeRequest{
		Args: messages.MustFromObject(body),
	}
	req.RequestID = utils.GenID(req.Args)
	req.User = strings.TrimSuffix(Name(ctx), "/local")
	taskR, err = NewTaskResolver(ctx, endpoint, req)
	if err != nil {
		return nil, err
	}
	<-taskR.Done()
	return taskR.Result()
}

// NewTaskResolver selects and returns resolver based on given context
func NewTaskResolver(ctx context.Context, endpoint string, request *messages.InvokeRequest) (TaskResolver, error) {
	if v := ctx.Value(natsKey); v != nil {
		natsConn := v.(*nats.Conn)
		return NewNatsResolver(ctx, natsConn, endpoint, request)
	}
	return NewHTTPResolver(ctx, endpoint, request)
}
