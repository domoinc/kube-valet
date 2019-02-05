package packleft

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	fakevalet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned/fake"
)

func fakeKeyFunc(obj interface{}) (string, error) {
	return "", nil
}

func TestGetNodePercentFullMemory(t *testing.T) {
	fakeIndexer := cache.NewIndexer(fakeKeyFunc, cache.Indexers{})
	m := NewManager(
		fakeIndexer,
		fakeIndexer,
		fakeIndexer,
		fakekube.NewSimpleClientset(),
		fakevalet.NewSimpleClientset(),
	)

	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testnode",
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{"memory": resource.MustParse("20Gi")},
			Allocatable: corev1.ResourceList{"memory": resource.MustParse("10Gi")},
		},
	}

	testCases := []struct {
		memUsed  []string
		expected float64
	}{
		{[]string{"0"}, 0.0},                        // Zero
		{[]string{"1Gi"}, 0.10},                     // Half with single
		{[]string{"256Mi", "256Mi", "512Mi"}, 0.10}, // Half with multiple, diverse
		{[]string{"10Gi"}, 1.0},                     // Full
	}

	for _, tc := range testCases {
		var testPods []*corev1.Pod
		for _, mem := range tc.memUsed {
			testPods = append(testPods, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "pod1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"memory": resource.MustParse(mem),
								},
							},
						},
					},
				},
			})
		}

		pFull := m.getNodePercentFullMemory(testNode, map[string][]*corev1.Pod{"testnode": testPods})

		if pFull != tc.expected {
			t.Errorf("Unexpected result: got %f; expected %f", pFull, tc.expected)
		}
	}
}

func TestGetNodePercentFullCPU(t *testing.T) {
	// No need for real indexers for this test
	fakeIndexer := cache.NewIndexer(fakeKeyFunc, cache.Indexers{})
	m := NewManager(
		fakeIndexer,
		fakeIndexer,
		fakeIndexer,
		fakekube.NewSimpleClientset(),
		fakevalet.NewSimpleClientset(),
	)

	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testnode",
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{"cpu": resource.MustParse("20")},
			Allocatable: corev1.ResourceList{"cpu": resource.MustParse("10")},
		},
	}

	testCases := []struct {
		cpuUsed  []string
		expected float64
	}{
		{[]string{"0"}, 0.0},      // Zero
		{[]string{"5"}, 0.5},      // Half with single
		{[]string{"1", "4"}, 0.5}, // Half with multiple, diverse
		{[]string{"10"}, 1.0},     // Full
	}

	for _, tc := range testCases {
		var testPods []*corev1.Pod
		for _, cpu := range tc.cpuUsed {
			testPods = append(testPods, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "pod1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu": resource.MustParse(cpu),
								},
							},
						},
					},
				},
			})
		}

		pFull := m.getNodePercentFullCPU(testNode, map[string][]*corev1.Pod{"testnode": testPods})

		if pFull != tc.expected {
			t.Errorf("Unexpected result: got %f; expected %f", pFull, tc.expected)
		}
	}
}
