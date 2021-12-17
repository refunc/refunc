/*
Copyright 2021 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta3 "github.com/refunc/refunc/pkg/generated/clientset/versioned/typed/refunc/v1beta3"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeRefuncV1beta3 struct {
	*testing.Fake
}

func (c *FakeRefuncV1beta3) Funcdeves(namespace string) v1beta3.FuncdefInterface {
	return &FakeFuncdeves{c, namespace}
}

func (c *FakeRefuncV1beta3) Funcinsts(namespace string) v1beta3.FuncinstInterface {
	return &FakeFuncinsts{c, namespace}
}

func (c *FakeRefuncV1beta3) Triggers(namespace string) v1beta3.TriggerInterface {
	return &FakeTriggers{c, namespace}
}

func (c *FakeRefuncV1beta3) Xenvs(namespace string) v1beta3.XenvInterface {
	return &FakeXenvs{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeRefuncV1beta3) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
