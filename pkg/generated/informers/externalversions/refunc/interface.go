/*
Copyright 2025 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by informer-gen. DO NOT EDIT.

package refunc

import (
	internalinterfaces "github.com/refunc/refunc/pkg/generated/informers/externalversions/internalinterfaces"
	v1beta3 "github.com/refunc/refunc/pkg/generated/informers/externalversions/refunc/v1beta3"
)

// Interface provides access to each of this group's versions.
type Interface interface {
	// V1beta3 provides access to shared informers for resources in V1beta3.
	V1beta3() v1beta3.Interface
}

type group struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &group{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// V1beta3 returns a new v1beta3.Interface.
func (g *group) V1beta3() v1beta3.Interface {
	return v1beta3.New(g.factory, g.namespace, g.tweakListOptions)
}
