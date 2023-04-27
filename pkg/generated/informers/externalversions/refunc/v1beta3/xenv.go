/*
Copyright 2023 The refunc Authors

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

// XenvInformer provides access to a shared informer and lister for
// Xenvs.
type XenvInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta3.XenvLister
}

type xenvInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewXenvInformer constructs a new informer for Xenv type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewXenvInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredXenvInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredXenvInformer constructs a new informer for Xenv type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredXenvInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.RefuncV1beta3().Xenvs(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.RefuncV1beta3().Xenvs(namespace).Watch(context.TODO(), options)
			},
		},
		&refuncv1beta3.Xenv{},
		resyncPeriod,
		indexers,
	)
}

func (f *xenvInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredXenvInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *xenvInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&refuncv1beta3.Xenv{}, f.defaultInformer)
}

func (f *xenvInformer) Lister() v1beta3.XenvLister {
	return v1beta3.NewXenvLister(f.Informer().GetIndexer())
}
