/*
Copyright 2025 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1beta3

import (
	v1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// FuncdefLister helps list Funcdeves.
// All objects returned here must be treated as read-only.
type FuncdefLister interface {
	// List lists all Funcdeves in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta3.Funcdef, err error)
	// Funcdeves returns an object that can list and get Funcdeves.
	Funcdeves(namespace string) FuncdefNamespaceLister
	FuncdefListerExpansion
}

// funcdefLister implements the FuncdefLister interface.
type funcdefLister struct {
	indexer cache.Indexer
}

// NewFuncdefLister returns a new FuncdefLister.
func NewFuncdefLister(indexer cache.Indexer) FuncdefLister {
	return &funcdefLister{indexer: indexer}
}

// List lists all Funcdeves in the indexer.
func (s *funcdefLister) List(selector labels.Selector) (ret []*v1beta3.Funcdef, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta3.Funcdef))
	})
	return ret, err
}

// Funcdeves returns an object that can list and get Funcdeves.
func (s *funcdefLister) Funcdeves(namespace string) FuncdefNamespaceLister {
	return funcdefNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// FuncdefNamespaceLister helps list and get Funcdeves.
// All objects returned here must be treated as read-only.
type FuncdefNamespaceLister interface {
	// List lists all Funcdeves in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta3.Funcdef, err error)
	// Get retrieves the Funcdef from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta3.Funcdef, error)
	FuncdefNamespaceListerExpansion
}

// funcdefNamespaceLister implements the FuncdefNamespaceLister
// interface.
type funcdefNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Funcdeves in the indexer for a given namespace.
func (s funcdefNamespaceLister) List(selector labels.Selector) (ret []*v1beta3.Funcdef, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta3.Funcdef))
	})
	return ret, err
}

// Get retrieves the Funcdef from the indexer for a given namespace and name.
func (s funcdefNamespaceLister) Get(name string) (*v1beta3.Funcdef, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1beta3.Resource("funcdef"), name)
	}
	return obj.(*v1beta3.Funcdef), nil
}
