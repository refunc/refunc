package loader

import "github.com/refunc/refunc/pkg/runtime/types"

// Loader discovers and loads function runtime config
type Loader interface {
	C() <-chan struct{}
	Function() *types.Function
	Setup()
}
