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

// FakeFuncinsts implements FuncinstInterface
type FakeFuncinsts struct {
	Fake *FakeRefuncV1beta3
	ns   string
}

var funcinstsResource = schema.GroupVersionResource{Group: "refunc.refunc.io", Version: "v1beta3", Resource: "funcinsts"}

var funcinstsKind = schema.GroupVersionKind{Group: "refunc.refunc.io", Version: "v1beta3", Kind: "Funcinst"}

// Get takes name of the funcinst, and returns the corresponding funcinst object, and an error if there is any.
func (c *FakeFuncinsts) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta3.Funcinst, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(funcinstsResource, c.ns, name), &v1beta3.Funcinst{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcinst), err
}

// List takes label and field selectors, and returns the list of Funcinsts that match those selectors.
func (c *FakeFuncinsts) List(ctx context.Context, opts v1.ListOptions) (result *v1beta3.FuncinstList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(funcinstsResource, funcinstsKind, c.ns, opts), &v1beta3.FuncinstList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta3.FuncinstList{ListMeta: obj.(*v1beta3.FuncinstList).ListMeta}
	for _, item := range obj.(*v1beta3.FuncinstList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested funcinsts.
func (c *FakeFuncinsts) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(funcinstsResource, c.ns, opts))

}

// Create takes the representation of a funcinst and creates it.  Returns the server's representation of the funcinst, and an error, if there is any.
func (c *FakeFuncinsts) Create(ctx context.Context, funcinst *v1beta3.Funcinst, opts v1.CreateOptions) (result *v1beta3.Funcinst, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(funcinstsResource, c.ns, funcinst), &v1beta3.Funcinst{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcinst), err
}

// Update takes the representation of a funcinst and updates it. Returns the server's representation of the funcinst, and an error, if there is any.
func (c *FakeFuncinsts) Update(ctx context.Context, funcinst *v1beta3.Funcinst, opts v1.UpdateOptions) (result *v1beta3.Funcinst, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(funcinstsResource, c.ns, funcinst), &v1beta3.Funcinst{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcinst), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeFuncinsts) UpdateStatus(ctx context.Context, funcinst *v1beta3.Funcinst, opts v1.UpdateOptions) (*v1beta3.Funcinst, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(funcinstsResource, "status", c.ns, funcinst), &v1beta3.Funcinst{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcinst), err
}

// Delete takes name of the funcinst and deletes it. Returns an error if one occurs.
func (c *FakeFuncinsts) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(funcinstsResource, c.ns, name, opts), &v1beta3.Funcinst{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFuncinsts) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(funcinstsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta3.FuncinstList{})
	return err
}

// Patch applies the patch and returns the patched funcinst.
func (c *FakeFuncinsts) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta3.Funcinst, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(funcinstsResource, c.ns, name, pt, data, subresources...), &v1beta3.Funcinst{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta3.Funcinst), err
}
