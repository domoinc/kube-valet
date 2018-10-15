package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	// ProtectedNodeLabelKey is used to tell kube-valet to never include a specific node in any NodeAssignmentGroups. Regardless of targeting.
	// Any node that is marked as protected will also have any existing NodeAssignmentGroup taints and labels removed.
	ProtectedNodeLabelKey = "nags.kube-valet.io/protected"

	// ProtectedLabelValue is the value that must be set on ProtectedNodeLabelKey for the node to be protected
	ProtectedLabelValue = "true"
)

// DeleteTaintsByKey removes all the taints that have the same key to given taintKey
func deleteTaintsByKey(taints []corev1.Taint, taintKey string) ([]corev1.Taint, bool) {
	newTaints := []corev1.Taint{}
	deleted := false
	for i := range taints {
		if taintKey == taints[i].Key {
			deleted = true
			continue
		}
		newTaints = append(newTaints, taints[i])
	}
	return newTaints, deleted
}

// DeleteTaint removes all the the taints that have the same key and effect to given taintToDelete.
func deleteTaint(taints []corev1.Taint, taintToDelete *corev1.Taint) ([]corev1.Taint, bool) {
	newTaints := []corev1.Taint{}
	deleted := false
	for i := range taints {
		if taintToDelete.MatchTaint(&taints[i]) {
			deleted = true
			continue
		}
		newTaints = append(newTaints, taints[i])
	}
	return newTaints, deleted
}

// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/taint.go#L140
// deleteTaints deletes the given taints from the node's taintlist.
func deleteTaints(taintsToRemove []corev1.Taint, newTaints *[]corev1.Taint) ([]error, bool) {
	allErrs := []error{}
	var removed bool
	for _, taintToRemove := range taintsToRemove {
		removed = false
		if len(taintToRemove.Effect) > 0 {
			*newTaints, removed = deleteTaint(*newTaints, &taintToRemove)
		} else {
			*newTaints, removed = deleteTaintsByKey(*newTaints, taintToRemove.Key)
		}
		if !removed {
			allErrs = append(allErrs, fmt.Errorf("taint %q not found", taintToRemove.ToString()))
		}
	}
	return allErrs, removed
}

func (nag *NodeAssignmentGroup) TargetsNode(node *corev1.Node) bool {
	// Nodes with the ProtectedNodeLabel set to true are never matched
	if val, ok := node.GetLabels()[ProtectedNodeLabelKey]; ok && val == ProtectedLabelValue {
		return false
	}

	// If there are no target labels than all nodes are targeted
	matched := true

	for k, v := range nag.Spec.TargetLabels {
		if val, ok := node.ObjectMeta.Labels[k]; ok {
			if v == val {
				matched = true
			} else {
				matched = false
				break
			}
		} else {
			matched = false
			break
		}
	}

	return matched
}

func (nag *NodeAssignmentGroup) GetAssignment(node *corev1.Node) (string, bool) {
	s, ok := node.ObjectMeta.Labels["nag."+GroupName+"/"+nag.ObjectMeta.Name]
	return s, ok
}

func (nag *NodeAssignmentGroup) SetLabel(node *corev1.Node, na *NodeAssignment) {
	// Generate assignment key from group and rule names
	key := "nag." + GroupName + "/" + nag.ObjectMeta.Name
	// Assignment
	node.ObjectMeta.Labels[key] = na.Name
}

func (nag *NodeAssignmentGroup) RemoveLabel(node *corev1.Node) {
	// Delete assignment Key
	// Ex: nag.assignments.kube-valet.io/NAGNAME
	delete(node.ObjectMeta.Labels, "nag."+GroupName+"/"+nag.ObjectMeta.Name)

	// Delete packleft key
	//nag.packleft.scheduling.kube-valet.io/NAGNAME
	delete(node.ObjectMeta.Labels, "nag.packleft.scheduling."+Domain+"/"+nag.ObjectMeta.Name)
}

func (nag *NodeAssignmentGroup) RemoveTaint(node *corev1.Node) []error {
	// Removing taint
	k := "nag." + GroupName + "/" + nag.ObjectMeta.Name
	plk := "nag.packleft.scheduling." + Domain + "/" + nag.ObjectMeta.Name

	// Remove all possible any taints for the group
	var removeTaints = []corev1.Taint{
		{Key: k, Effect: corev1.TaintEffectNoSchedule},
		{Key: k, Effect: corev1.TaintEffectPreferNoSchedule},
		{Key: k, Effect: corev1.TaintEffectNoExecute},
		{Key: plk, Effect: corev1.TaintEffectNoSchedule},
		{Key: plk, Effect: corev1.TaintEffectPreferNoSchedule},
		{Key: plk, Effect: corev1.TaintEffectNoExecute},
	}
	allErrs, _ := deleteTaints(removeTaints, &node.Spec.Taints)

	return allErrs
}

func (nag *NodeAssignmentGroup) SetTaint(node *corev1.Node, na *NodeAssignment) {
	// Set Taint
	// Generate taint key from group and rule names
	key := "nag." + GroupName + "/" + nag.ObjectMeta.Name

	if na.TaintEffect == NodeAssignmentTaintEffectNotSpecified {
		na.TaintEffect = NodeAssignmentTaintEffectDefault
	}

	// Assignment
	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
		Key:    key,
		Value:  na.Name,
		Effect: na.TaintEffect,
	})
}

func (nag *NodeAssignmentGroup) Assign(node *corev1.Node, na *NodeAssignment) {
	// If the user didn't define a mode, Use the default
	if na.Mode == NodeAssignmentModeUndefined {
		na.Mode = NodeAssignmentModeDefault
	}

	// Label And/Or Taint based on the assignment's rule
	switch na.Mode {
	case NodeAssignmentModeLabelAndTaint:
		nag.SetLabel(node, na)
		nag.SetTaint(node, na)
	case NodeAssignmentModeLabelOnly:
		nag.SetLabel(node, na)
	}
}

func (s *PodAssignmentRuleSpec) TargetsPod(pod *corev1.Pod) bool {
	// If there are no target labels than all pods are targeted
	matched := true

	for k, v := range s.TargetLabels {
		if val, ok := pod.ObjectMeta.Labels[k]; ok {
			if v == val {
				matched = true
			} else {
				matched = false
				break
			}
		} else {
			matched = false
			break
		}
	}

	return matched
}

func (r *PodAssignmentRule) TargetsPod(pod *corev1.Pod) bool {
	return r.Spec.TargetsPod(pod)
}

func (r *ClusterPodAssignmentRule) TargetsPod(pod *corev1.Pod) bool {
	return r.Spec.TargetsPod(pod)
}

func (nag *NodeAssignmentGroup) Unassign(node *corev1.Node) []error {
	nag.RemoveLabel(node)
	return nag.RemoveTaint(node)
}

func (s *PodAssignmentRuleScheduling) GetMergeStrategy() PodAssignmentRuleSchedulingMergeStrategy {
	if s.MergeStrategy == PodAssignmentRuleSchedulingMergeStrategyUndefined {
		return PodAssignmentRuleSchedulingMergeStrategyDefault
	}
	return s.MergeStrategy
}

func (s *PodAssignmentRuleScheduling) ApplyToPod(pod *corev1.Pod) {
	// Support all known merge strategies
	switch ms := s.GetMergeStrategy(); ms {
	case PodAssignmentRuleSchedulingMergeStrategyOverwriteAll:
		if len(s.NodeSelector) != 0 {
			pod.Spec.NodeSelector = s.NodeSelector
		}
		if s.Affinity != nil {
			pod.Spec.Affinity = s.Affinity
		}
		if len(s.Tolerations) != 0 {
			pod.Spec.Tolerations = s.Tolerations
		}
	}
}
