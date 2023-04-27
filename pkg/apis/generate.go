//go:build generate
// +build generate

package apis

//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen crd paths=./... output:artifacts:config=../../crds

import (
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen" //nolint:typecheck
)
