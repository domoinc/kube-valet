/*
Copyright 2018 The Kubernetes Authors.

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

package fake

import (
	v1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeNodeAssignmentGroups implements NodeAssignmentGroupInterface
type FakeNodeAssignmentGroups struct {
	Fake *FakeAssignmentsV1alpha1
}

var nodeassignmentgroupsResource = schema.GroupVersionResource{Group: "assignments.kube-valet.io", Version: "v1alpha1", Resource: "nodeassignmentgroups"}

var nodeassignmentgroupsKind = schema.GroupVersionKind{Group: "assignments.kube-valet.io", Version: "v1alpha1", Kind: "NodeAssignmentGroup"}

// Get takes name of the nodeAssignmentGroup, and returns the corresponding nodeAssignmentGroup object, and an error if there is any.
func (c *FakeNodeAssignmentGroups) Get(name string, options v1.GetOptions) (result *v1alpha1.NodeAssignmentGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(nodeassignmentgroupsResource, name), &v1alpha1.NodeAssignmentGroup{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NodeAssignmentGroup), err
}

// List takes label and field selectors, and returns the list of NodeAssignmentGroups that match those selectors.
func (c *FakeNodeAssignmentGroups) List(opts v1.ListOptions) (result *v1alpha1.NodeAssignmentGroupList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(nodeassignmentgroupsResource, nodeassignmentgroupsKind, opts), &v1alpha1.NodeAssignmentGroupList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.NodeAssignmentGroupList{}
	for _, item := range obj.(*v1alpha1.NodeAssignmentGroupList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested nodeAssignmentGroups.
func (c *FakeNodeAssignmentGroups) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(nodeassignmentgroupsResource, opts))
}

// Create takes the representation of a nodeAssignmentGroup and creates it.  Returns the server's representation of the nodeAssignmentGroup, and an error, if there is any.
func (c *FakeNodeAssignmentGroups) Create(nodeAssignmentGroup *v1alpha1.NodeAssignmentGroup) (result *v1alpha1.NodeAssignmentGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(nodeassignmentgroupsResource, nodeAssignmentGroup), &v1alpha1.NodeAssignmentGroup{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NodeAssignmentGroup), err
}

// Update takes the representation of a nodeAssignmentGroup and updates it. Returns the server's representation of the nodeAssignmentGroup, and an error, if there is any.
func (c *FakeNodeAssignmentGroups) Update(nodeAssignmentGroup *v1alpha1.NodeAssignmentGroup) (result *v1alpha1.NodeAssignmentGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(nodeassignmentgroupsResource, nodeAssignmentGroup), &v1alpha1.NodeAssignmentGroup{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NodeAssignmentGroup), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeNodeAssignmentGroups) UpdateStatus(nodeAssignmentGroup *v1alpha1.NodeAssignmentGroup) (*v1alpha1.NodeAssignmentGroup, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(nodeassignmentgroupsResource, "status", nodeAssignmentGroup), &v1alpha1.NodeAssignmentGroup{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NodeAssignmentGroup), err
}

// Delete takes name of the nodeAssignmentGroup and deletes it. Returns an error if one occurs.
func (c *FakeNodeAssignmentGroups) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(nodeassignmentgroupsResource, name), &v1alpha1.NodeAssignmentGroup{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeNodeAssignmentGroups) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(nodeassignmentgroupsResource, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.NodeAssignmentGroupList{})
	return err
}

// Patch applies the patch and returns the patched nodeAssignmentGroup.
func (c *FakeNodeAssignmentGroups) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NodeAssignmentGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(nodeassignmentgroupsResource, name, data, subresources...), &v1alpha1.NodeAssignmentGroup{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NodeAssignmentGroup), err
}
