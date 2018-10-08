/*
Copyright 2017 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

const (
	Domain = "kube-valet.io"

	GroupName = "assignments.kube-valet.io"
	V1alpha1  = "v1alpha1"

	PodAssignmentRuleResourceKind       = "PodAssignmentRule"
	PodAssignmentRuleResourceName       = "podassignmentrule"
	PodAssignmentRuleResourceNamePlural = "podassignmentrules"

	ClusterPodAssignmentRuleResourceKind       = "ClusterPodAssignmentRule"
	ClusterPodAssignmentRuleResourceName       = "clusterpodassignmentrule"
	ClusterPodAssignmentRuleResourceNamePlural = "clusterpodassignmentrules"

	NodeAssignmentGroupResourceKind       = "NodeAssignmentGroup"
	NodeAssignmentGroupResourceName       = "nodeassignmentgroup"
	NodeAssignmentGroupResourceNamePlural = "nodeassignmentgroups"
)

var (
	// SchemeGroupVersion is the group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: V1alpha1}

	ClusterPodAssignmentRuleCRDName = ClusterPodAssignmentRuleResourceNamePlural + "." + GroupName
	NodeAssignmentGroupCRDName      = NodeAssignmentGroupResourceNamePlural + "." + GroupName
)

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group-qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// addKnownTypes adds the set of types defined in this package to the supplied scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&PodAssignmentRule{},
		&PodAssignmentRuleList{},
		&ClusterPodAssignmentRule{},
		&ClusterPodAssignmentRuleList{},
		&NodeAssignmentGroup{},
		&NodeAssignmentGroupList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
