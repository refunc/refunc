package transport

import (
	"context"
	"sync"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/operators"
	corev1 "k8s.io/api/core/v1"
)

// Namer returns the name of transport related resource
type Namer interface {
	Name() string
}

// SidecarProvider returns a sidecard container for a transport
type SidecarProvider interface {
	Namer

	GetTransportContainer(tpl *rfv1beta3.Xenv) *corev1.Container
}

// OperatorHandler creates funcinst and forwards messages
type OperatorHandler interface {
	Namer

	Start(ctx context.Context, operator operators.Interface)
}

var registry struct {
	sync.Mutex
	sidecars map[string]SidecarProvider // runtime instances
}

// Register adds a new versioned runner to registry
func Register(r SidecarProvider) {
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.sidecars[r.Name()]; !ok {
		registry.sidecars[r.Name()] = r
	}
}

// ForXenv returns runtime object for given xenv
func ForXenv(xenv *rfv1beta3.Xenv) SidecarProvider {
	registry.Lock()
	defer registry.Unlock()
	typ := xenv.Spec.Transport
	if r, ok := registry.sidecars[typ]; ok {
		return r
	}
	panic("Unsupported transport: " + typ)
}

type emptyProvider struct{}

func (emptyProvider) Name() string                                                { return "" }
func (emptyProvider) GetTransportContainer(tpl *rfv1beta3.Xenv) *corev1.Container { return nil }

func init() {
	registry.sidecars = map[string]SidecarProvider{
		"": emptyProvider{},
	}
}
