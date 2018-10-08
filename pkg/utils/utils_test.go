package utils

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var patchtests = []struct {
	p string
	r bool
}{
	{"{}", true},
	{"", true},
	{"{patchdata}", false},
}

func TestIsEmptyPach(t *testing.T) {
	for _, tt := range patchtests {
		t.Run(tt.p, func(t *testing.T) {
			bs := []byte(tt.p)
			r := IsEmptyPatch(bs)
			if r != tt.r {
				t.Errorf("got %v, want %v", r, tt.r)
			}
		})
	}
}

var labeltests = []struct {
	name string
	m1   map[string]string
	m2   map[string]string
	r    bool
}{
	{
		"Unchanged",
		map[string]string{"target1": "value1"},
		map[string]string{"target1": "value1"},
		false,
	},
	{
		"UnchangedIgnoreValetLabels",
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign1"},
		map[string]string{"target1": "value1", "nag.packleft.scheduling.kube-valet.io/packleft1": "assign1"},
		false,
	},
	{
		"UnchangedIgnoreValetLabelAdd",
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign1"},
		map[string]string{"target1": "value1"},
		false,
	},
	{
		"UnchangedIgnoreValetLabelChange",
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign1"},
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign2"},
		false,
	},
	{
		"UnchangedIgnoreValetLabelChange",
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign1"},
		map[string]string{"target1": "value1", "nag.assignments.kube-valet.io/nag1": "assign2"},
		false,
	},
	{
		"ValueChange",
		map[string]string{"target1": "value1"},
		map[string]string{"target1": "value2"},
		true,
	},
	{
		"KeyChange",
		map[string]string{"target1": "value1"},
		map[string]string{"target2": "value1"},
		true,
	},
	{
		"KeyAdd",
		map[string]string{"target1": "value1"},
		map[string]string{"target1": "value1", "target2": "value2"},
		true,
	},
	{
		"KeyRemove",
		map[string]string{"target1": "value1"},
		map[string]string{},
		true,
	},
}

func TestTargetableLabelChanges(t *testing.T) {
	for _, tt := range labeltests {
		t.Run(tt.name, func(t *testing.T) {
			r := TargetableLabelsAreDifferent(tt.m1, tt.m2)
			if r != tt.r {
				t.Errorf("got %v, want %v", r, tt.r)
			}
		})
	}
}

// Use the same label table, just make nodes with them first
func TestNodeTargetingHasChanged(t *testing.T) {
	for _, tt := range labeltests {
		t.Run(tt.name, func(t *testing.T) {
			oldNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "oldNode",
					Labels: tt.m1,
				},
			}
			newNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "newNode",
					Labels: tt.m2,
				},
			}
			r := NodeTargetingHasChanged(oldNode, newNode)
			if r != tt.r {
				t.Errorf("got %v, want %v", r, tt.r)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	o := []string{"keep", "filterout", "keep"}
	n := []string{"keep", "keep"}

	r := FilterValues(o, func(s string) bool {
		if s == "filterout" {
			return false
		}
		return true
	})

	if !reflect.DeepEqual(r, n) {
		t.Errorf("got %v, want %v", r, n)
	}
}
