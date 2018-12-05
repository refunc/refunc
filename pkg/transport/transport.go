package transport

import (
	"context"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/operators"
	corev1 "k8s.io/api/core/v1"
)

// Namer returns the name of transport related resource
type Namer interface {
	Name() string
}

// A Interface interface
type Interface interface {
	Namer

	GetTransportContainer(tpl *rfv1beta3.Xenv) *corev1.Container

	Start(ctx context.Context, operator operators.Interface)
}

// OperatorHandler creates funcinst and forwards messages
type OperatorHandler interface {
	Namer

	Start(ctx context.Context, operator operators.Interface)
}
