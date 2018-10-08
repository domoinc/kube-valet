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

package v1alpha1

import (
	v1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	scheme "github.com/domoinc/kube-valet/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// PodAssignmentRulesGetter has a method to return a PodAssignmentRuleInterface.
// A group's client should implement this interface.
type PodAssignmentRulesGetter interface {
	PodAssignmentRules(namespace string) PodAssignmentRuleInterface
}

// PodAssignmentRuleInterface has methods to work with PodAssignmentRule resources.
type PodAssignmentRuleInterface interface {
	Create(*v1alpha1.PodAssignmentRule) (*v1alpha1.PodAssignmentRule, error)
	Update(*v1alpha1.PodAssignmentRule) (*v1alpha1.PodAssignmentRule, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.PodAssignmentRule, error)
	List(opts v1.ListOptions) (*v1alpha1.PodAssignmentRuleList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.PodAssignmentRule, err error)
	PodAssignmentRuleExpansion
}

// podAssignmentRules implements PodAssignmentRuleInterface
type podAssignmentRules struct {
	client rest.Interface
	ns     string
}

// newPodAssignmentRules returns a PodAssignmentRules
func newPodAssignmentRules(c *AssignmentsV1alpha1Client, namespace string) *podAssignmentRules {
	return &podAssignmentRules{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the podAssignmentRule, and returns the corresponding podAssignmentRule object, and an error if there is any.
func (c *podAssignmentRules) Get(name string, options v1.GetOptions) (result *v1alpha1.PodAssignmentRule, err error) {
	result = &v1alpha1.PodAssignmentRule{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("podassignmentrules").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of PodAssignmentRules that match those selectors.
func (c *podAssignmentRules) List(opts v1.ListOptions) (result *v1alpha1.PodAssignmentRuleList, err error) {
	result = &v1alpha1.PodAssignmentRuleList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("podassignmentrules").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested podAssignmentRules.
func (c *podAssignmentRules) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("podassignmentrules").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a podAssignmentRule and creates it.  Returns the server's representation of the podAssignmentRule, and an error, if there is any.
func (c *podAssignmentRules) Create(podAssignmentRule *v1alpha1.PodAssignmentRule) (result *v1alpha1.PodAssignmentRule, err error) {
	result = &v1alpha1.PodAssignmentRule{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("podassignmentrules").
		Body(podAssignmentRule).
		Do().
		Into(result)
	return
}

// Update takes the representation of a podAssignmentRule and updates it. Returns the server's representation of the podAssignmentRule, and an error, if there is any.
func (c *podAssignmentRules) Update(podAssignmentRule *v1alpha1.PodAssignmentRule) (result *v1alpha1.PodAssignmentRule, err error) {
	result = &v1alpha1.PodAssignmentRule{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("podassignmentrules").
		Name(podAssignmentRule.Name).
		Body(podAssignmentRule).
		Do().
		Into(result)
	return
}

// Delete takes name of the podAssignmentRule and deletes it. Returns an error if one occurs.
func (c *podAssignmentRules) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("podassignmentrules").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *podAssignmentRules) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("podassignmentrules").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched podAssignmentRule.
func (c *podAssignmentRules) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.PodAssignmentRule, err error) {
	result = &v1alpha1.PodAssignmentRule{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("podassignmentrules").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
