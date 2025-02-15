/*
Copyright 2025 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1beta3

import (
	"context"
	time "time"

	refuncv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	versioned "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/refunc/refunc/pkg/generated/informers/externalversions/internalinterfaces"
	v1beta3 "github.com/refunc/refunc/pkg/generated/listers/refunc/v1beta3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// FuncdefInformer provides access to a shared informer and lister for
// Funcdeves.
type FuncdefInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta3.FuncdefLister
}

type funcdefInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewFuncdefInformer constructs a new informer for Funcdef type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFuncdefInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredFuncdefInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredFuncdefInformer constructs a new informer for Funcdef type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredFuncdefInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.RefuncV1beta3().Funcdeves(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.RefuncV1beta3().Funcdeves(namespace).Watch(context.TODO(), options)
			},
		},
		&refuncv1beta3.Funcdef{},
		resyncPeriod,
		indexers,
	)
}

func (f *funcdefInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredFuncdefInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *funcdefInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&refuncv1beta3.Funcdef{}, f.defaultInformer)
}

func (f *funcdefInformer) Lister() v1beta3.FuncdefLister {
	return v1beta3.NewFuncdefLister(f.Informer().GetIndexer())
}
