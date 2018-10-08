package controller

import (
	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// NodeController a controller that processes node events
type NodeController interface {
	OnAddNode(node *corev1.Node)
	OnUpdateNode(oldNode *corev1.Node, newNode *corev1.Node)
	OnDeleteNode(node *corev1.Node)
}

// NagController a controller that processes nag events
type NagController interface {
	OnAddNag(nag *assignmentsv1alpha1.NodeAssignmentGroup)
	OnUpdateNag(oldNag *assignmentsv1alpha1.NodeAssignmentGroup, newNag *assignmentsv1alpha1.NodeAssignmentGroup)
	OnDeleteNag(nag *assignmentsv1alpha1.NodeAssignmentGroup)
}

// PodController a controller that processes pod events
type PodController interface {
	OnAddPod(pod *corev1.Pod)
	OnUpdatePod(oldPod *corev1.Pod, newPod *corev1.Pod)
	OnDeletePod(pod *corev1.Pod)
}
