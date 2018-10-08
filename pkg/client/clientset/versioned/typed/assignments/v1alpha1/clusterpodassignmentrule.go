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

// ClusterPodAssignmentRulesGetter has a method to return a ClusterPodAssignmentRuleInterface.
// A group's client should implement this interface.
type ClusterPodAssignmentRulesGetter interface {
	ClusterPodAssignmentRules() ClusterPodAssignmentRuleInterface
}

// ClusterPodAssignmentRuleInterface has methods to work with ClusterPodAssignmentRule resources.
type ClusterPodAssignmentRuleInterface interface {
	Create(*v1alpha1.ClusterPodAssignmentRule) (*v1alpha1.ClusterPodAssignmentRule, error)
	Update(*v1alpha1.ClusterPodAssignmentRule) (*v1alpha1.ClusterPodAssignmentRule, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.ClusterPodAssignmentRule, error)
	List(opts v1.ListOptions) (*v1alpha1.ClusterPodAssignmentRuleList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterPodAssignmentRule, err error)
	ClusterPodAssignmentRuleExpansion
}

// clusterPodAssignmentRules implements ClusterPodAssignmentRuleInterface
type clusterPodAssignmentRules struct {
	client rest.Interface
}

// newClusterPodAssignmentRules returns a ClusterPodAssignmentRules
func newClusterPodAssignmentRules(c *AssignmentsV1alpha1Client) *clusterPodAssignmentRules {
	return &clusterPodAssignmentRules{
		client: c.RESTClient(),
	}
}

// Get takes name of the clusterPodAssignmentRule, and returns the corresponding clusterPodAssignmentRule object, and an error if there is any.
func (c *clusterPodAssignmentRules) Get(name string, options v1.GetOptions) (result *v1alpha1.ClusterPodAssignmentRule, err error) {
	result = &v1alpha1.ClusterPodAssignmentRule{}
	err = c.client.Get().
		Resource("clusterpodassignmentrules").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterPodAssignmentRules that match those selectors.
func (c *clusterPodAssignmentRules) List(opts v1.ListOptions) (result *v1alpha1.ClusterPodAssignmentRuleList, err error) {
	result = &v1alpha1.ClusterPodAssignmentRuleList{}
	err = c.client.Get().
		Resource("clusterpodassignmentrules").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterPodAssignmentRules.
func (c *clusterPodAssignmentRules) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Resource("clusterpodassignmentrules").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a clusterPodAssignmentRule and creates it.  Returns the server's representation of the clusterPodAssignmentRule, and an error, if there is any.
func (c *clusterPodAssignmentRules) Create(clusterPodAssignmentRule *v1alpha1.ClusterPodAssignmentRule) (result *v1alpha1.ClusterPodAssignmentRule, err error) {
	result = &v1alpha1.ClusterPodAssignmentRule{}
	err = c.client.Post().
		Resource("clusterpodassignmentrules").
		Body(clusterPodAssignmentRule).
		Do().
		Into(result)
	return
}

// Update takes the representation of a clusterPodAssignmentRule and updates it. Returns the server's representation of the clusterPodAssignmentRule, and an error, if there is any.
func (c *clusterPodAssignmentRules) Update(clusterPodAssignmentRule *v1alpha1.ClusterPodAssignmentRule) (result *v1alpha1.ClusterPodAssignmentRule, err error) {
	result = &v1alpha1.ClusterPodAssignmentRule{}
	err = c.client.Put().
		Resource("clusterpodassignmentrules").
		Name(clusterPodAssignmentRule.Name).
		Body(clusterPodAssignmentRule).
		Do().
		Into(result)
	return
}

// Delete takes name of the clusterPodAssignmentRule and deletes it. Returns an error if one occurs.
func (c *clusterPodAssignmentRules) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusterpodassignmentrules").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterPodAssignmentRules) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Resource("clusterpodassignmentrules").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched clusterPodAssignmentRule.
func (c *clusterPodAssignmentRules) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterPodAssignmentRule, err error) {
	result = &v1alpha1.ClusterPodAssignmentRule{}
	err = c.client.Patch(pt).
		Resource("clusterpodassignmentrules").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
