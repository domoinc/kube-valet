package podassignment

import (
	// "encoding/json"

	// "github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	// "k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	"github.com/domoinc/kube-valet/pkg/utils"
)

type Manager struct {
	log        *logging.Logger
	podIndex   cache.Indexer
	cparIndex  cache.Indexer
	parIndex   cache.Indexer
	kubeClient kubernetes.Interface
}

func NewManager(podIndex cache.Indexer, cparIndex cache.Indexer, parIndex cache.Indexer, kubeClient kubernetes.Interface) *Manager {
	return &Manager{
		log:        logging.MustGetLogger("PodAssignmentManager"),
		podIndex:   podIndex,
		cparIndex:  cparIndex,
		parIndex:   parIndex,
		kubeClient: kubeClient,
	}
}

func (m *Manager) PodIsProtected(pod *corev1.Pod) bool {
	for k, v := range pod.GetLabels() {
		if k == ProtectedLabelKey && v == ProtectedLabelValue {
			return true
		}
	}
	return false
}

// TODO make this ordered!
func (m *Manager) GetPodAssignmentsScheduling(pod *corev1.Pod) []*assignmentsv1alpha1.PodAssignmentRuleScheduling {
	var r []*assignmentsv1alpha1.PodAssignmentRuleScheduling

	// Should probably not copy rules for every pod. But it's more dangerous to point to rules in memory since the underlying objects might change

	// Non-Namespaced, get all in store
	if err := cache.ListAll(m.cparIndex, labels.Everything(), func(obj interface{}) {
		if obj.(*assignmentsv1alpha1.ClusterPodAssignmentRule).TargetsPod(pod) {
			r = append(r, obj.(*assignmentsv1alpha1.ClusterPodAssignmentRule).Spec.Scheduling.DeepCopy())
		}
	}); err != nil {
		m.log.Errorf("Unable to get Non-Namespaced pod assignment scheduling %s", err)
	}

	// Namespaced, get via indexer
	if err := cache.ListAllByNamespace(m.parIndex, pod.GetNamespace(), labels.Everything(), func(obj interface{}) {
		if obj.(*assignmentsv1alpha1.PodAssignmentRule).TargetsPod(pod) {
			r = append(r, obj.(*assignmentsv1alpha1.PodAssignmentRule).Spec.Scheduling.DeepCopy())
		}
	}); err != nil {
		m.log.Errorf("Unable to get Namespaced pod assignment scheduling %s", err)
	}

	return r
}

func (m *Manager) GetPodSchedulingPatches(pod *corev1.Pod) []utils.JsonPatchOperation {
	m.log.Debugf("Generating schedule patches for pod in %s", pod.GetNamespace())

	patchOps := []utils.JsonPatchOperation{}

	if !m.PodIsProtected(pod) {
		// Figure out which assignments this pod matches
		scheds := m.GetPodAssignmentsScheduling(pod)

		m.log.Debugf("Matched %d scheduling rule(s)", len(scheds))

		// Append all patch operations
		for _, s := range scheds {
			patchOps = append(patchOps, s.GetPatchOps(pod)...)
		}
	}

	return patchOps
}
