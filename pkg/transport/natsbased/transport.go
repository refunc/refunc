package natsbased

import (
	"context"
	"sync"

	nats "github.com/nats-io/nats.go"
	"github.com/refunc/refunc/pkg/operators"
	"github.com/refunc/refunc/pkg/transport"
)

type natsHandler struct {
	operator operators.Interface

	natsConn *nats.Conn

	ctx    context.Context
	cancel context.CancelFunc

	cryMap sync.Map
}

// NewHandler creates a nats based transport.OperatorHandler
func NewHandler(conn *nats.Conn) transport.OperatorHandler {
	return &natsHandler{
		natsConn: conn,
	}
}

func (nh *natsHandler) Name() string {
	return "nats"
}
