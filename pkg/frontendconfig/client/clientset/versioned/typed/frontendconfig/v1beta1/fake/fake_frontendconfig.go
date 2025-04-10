/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1beta1 "k8s.io/ingress-gce/pkg/apis/frontendconfig/v1beta1"
)

// FakeFrontendConfigs implements FrontendConfigInterface
type FakeFrontendConfigs struct {
	Fake *FakeNetworkingV1beta1
	ns   string
}

var frontendconfigsResource = schema.GroupVersionResource{Group: "networking.gke.io", Version: "v1beta1", Resource: "frontendconfigs"}

var frontendconfigsKind = schema.GroupVersionKind{Group: "networking.gke.io", Version: "v1beta1", Kind: "FrontendConfig"}

// Get takes name of the frontendConfig, and returns the corresponding frontendConfig object, and an error if there is any.
func (c *FakeFrontendConfigs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta1.FrontendConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(frontendconfigsResource, c.ns, name), &v1beta1.FrontendConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.FrontendConfig), err
}

// List takes label and field selectors, and returns the list of FrontendConfigs that match those selectors.
func (c *FakeFrontendConfigs) List(ctx context.Context, opts v1.ListOptions) (result *v1beta1.FrontendConfigList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(frontendconfigsResource, frontendconfigsKind, c.ns, opts), &v1beta1.FrontendConfigList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.FrontendConfigList{ListMeta: obj.(*v1beta1.FrontendConfigList).ListMeta}
	for _, item := range obj.(*v1beta1.FrontendConfigList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested frontendConfigs.
func (c *FakeFrontendConfigs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(frontendconfigsResource, c.ns, opts))

}

// Create takes the representation of a frontendConfig and creates it.  Returns the server's representation of the frontendConfig, and an error, if there is any.
func (c *FakeFrontendConfigs) Create(ctx context.Context, frontendConfig *v1beta1.FrontendConfig, opts v1.CreateOptions) (result *v1beta1.FrontendConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(frontendconfigsResource, c.ns, frontendConfig), &v1beta1.FrontendConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.FrontendConfig), err
}

// Update takes the representation of a frontendConfig and updates it. Returns the server's representation of the frontendConfig, and an error, if there is any.
func (c *FakeFrontendConfigs) Update(ctx context.Context, frontendConfig *v1beta1.FrontendConfig, opts v1.UpdateOptions) (result *v1beta1.FrontendConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(frontendconfigsResource, c.ns, frontendConfig), &v1beta1.FrontendConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.FrontendConfig), err
}

// Delete takes name of the frontendConfig and deletes it. Returns an error if one occurs.
func (c *FakeFrontendConfigs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(frontendconfigsResource, c.ns, name), &v1beta1.FrontendConfig{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFrontendConfigs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(frontendconfigsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta1.FrontendConfigList{})
	return err
}

// Patch applies the patch and returns the patched frontendConfig.
func (c *FakeFrontendConfigs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.FrontendConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(frontendconfigsResource, c.ns, name, pt, data, subresources...), &v1beta1.FrontendConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.FrontendConfig), err
}
