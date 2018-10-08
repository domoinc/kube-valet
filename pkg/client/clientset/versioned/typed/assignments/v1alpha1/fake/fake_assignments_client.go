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
	v1alpha1 "github.com/domoinc/kube-valet/pkg/client/clientset/versioned/typed/assignments/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeAssignmentsV1alpha1 struct {
	*testing.Fake
}

func (c *FakeAssignmentsV1alpha1) ClusterPodAssignmentRules() v1alpha1.ClusterPodAssignmentRuleInterface {
	return &FakeClusterPodAssignmentRules{c}
}

func (c *FakeAssignmentsV1alpha1) NodeAssignmentGroups() v1alpha1.NodeAssignmentGroupInterface {
	return &FakeNodeAssignmentGroups{c}
}

func (c *FakeAssignmentsV1alpha1) PodAssignmentRules(namespace string) v1alpha1.PodAssignmentRuleInterface {
	return &FakePodAssignmentRules{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeAssignmentsV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
