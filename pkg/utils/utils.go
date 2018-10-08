package utils

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// IsEmptyPatch returns true if a generate patch is empty
func IsEmptyPatch(b []byte) bool {
	// an empty patch of "{}" has a len of 2. No need to send a zero patch
	// also report true on zero len slice
	if len(b) == 2 || len(b) == 0 {
		return true
	}
	return false
}

// TargetableLabelsAreDifferent compares two maps and looks for changes, But ignores any keys that are
// applied by kube-valet to avoid loops.
func TargetableLabelsAreDifferent(m1 map[string]string, m2 map[string]string) bool {
	// Loop m1 items
	for k, v1 := range m1 {
		// Ignore if key starts with the kube-valet prefix
		if strings.Contains(k, "kube-valet.io") {
			continue
		}
		// if the key isn't in m2 or doesn't have the same value then the labels are different
		if v2, ok := m2[k]; !ok || ok && v1 != v2 {
			return true
		}
	}
	// Loop m2 items
	for k, v2 := range m2 {
		// Ignore if key starts with the kube-valet prefix
		if strings.Contains(k, "kube-valet.io") {
			continue
		}
		// if the key isn't in m1 or doesn't have the same value then the labels are different
		if v1, ok := m1[k]; !ok || ok && v1 != v2 {
			return true
		}
	}
	return false
}

// NodeTargetingHasChanged checks for changes in targetable aspects between two nodes
func NodeTargetingHasChanged(oldNode *corev1.Node, newNode *corev1.Node) bool {
	if TargetableLabelsAreDifferent(oldNode.GetLabels(), newNode.GetLabels()) {
		return true
	}
	return false
}

// FilterValues Takes a slice of strings and returns a new slice that is filtered by the passed function
func FilterValues(vs []string, f func(string) bool) []string {
	var vsf []string
	for _, v := range vs {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}
