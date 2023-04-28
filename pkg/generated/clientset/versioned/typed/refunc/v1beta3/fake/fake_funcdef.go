/*
Copyright 2023 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeFuncdeves implements FuncdefInterface
type FakeFuncdeves struct {
	Fake *FakeRefuncV1beta3
	ns   string
}

var funcdevesResource = schema.GroupVersionResource{Group: "refunc.refunc.io", Version: "v1beta3", Resource: "funcdeves"}

var funcdevesKind = schema.GroupVersionKind{Group: "refunc.refunc.io", Version: "v1beta3", Kind: "Funcdef"}

// Get takes name of the funcdef, and returns the corresponding funcdef object, and an error if there is any.
func (c *FakeFuncdeves) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta3.Funcdef, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(funcdevesResource, c.ns, name), &v1beta3.Funcdef{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcdef), err
}

// List takes label and field selectors, and returns the list of Funcdeves that match those selectors.
func (c *FakeFuncdeves) List(ctx context.Context, opts v1.ListOptions) (result *v1beta3.FuncdefList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(funcdevesResource, funcdevesKind, c.ns, opts), &v1beta3.FuncdefList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta3.FuncdefList{ListMeta: obj.(*v1beta3.FuncdefList).ListMeta}
	for _, item := range obj.(*v1beta3.FuncdefList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested funcdeves.
func (c *FakeFuncdeves) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(funcdevesResource, c.ns, opts))

}

// Create takes the representation of a funcdef and creates it.  Returns the server's representation of the funcdef, and an error, if there is any.
func (c *FakeFuncdeves) Create(ctx context.Context, funcdef *v1beta3.Funcdef, opts v1.CreateOptions) (result *v1beta3.Funcdef, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(funcdevesResource, c.ns, funcdef), &v1beta3.Funcdef{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcdef), err
}

// Update takes the representation of a funcdef and updates it. Returns the server's representation of the funcdef, and an error, if there is any.
func (c *FakeFuncdeves) Update(ctx context.Context, funcdef *v1beta3.Funcdef, opts v1.UpdateOptions) (result *v1beta3.Funcdef, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(funcdevesResource, c.ns, funcdef), &v1beta3.Funcdef{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcdef), err
}

// Delete takes name of the funcdef and deletes it. Returns an error if one occurs.
func (c *FakeFuncdeves) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(funcdevesResource, c.ns, name, opts), &v1beta3.Funcdef{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFuncdeves) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(funcdevesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta3.FuncdefList{})
	return err
}

// Patch applies the patch and returns the patched funcdef.
func (c *FakeFuncdeves) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta3.Funcdef, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(funcdevesResource, c.ns, name, pt, data, subresources...), &v1beta3.Funcdef{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcdef), err
}
