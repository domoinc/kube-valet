package podassignment

import (
	"encoding/json"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	"github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtime_pkg "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type Manager struct {
	log           *logging.Logger
	podIndex      cache.Indexer
	cparIndex     cache.Indexer
	parIndex      cache.Indexer
	kubeClientset *kubernetes.Clientset
}

func NewManager(podIndex cache.Indexer, cparIndex cache.Indexer, parIndex cache.Indexer, kubeClientset *kubernetes.Clientset) *Manager {
	return &Manager{
		log:           logging.MustGetLogger("PodAssignmentManager"),
		podIndex:      podIndex,
		cparIndex:     cparIndex,
		parIndex:      parIndex,
		kubeClientset: kubeClientset,
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

func (m *Manager) InitializePod(pod *corev1.Pod) error {

	m.log.Debugf("Initializing pod: %s", pod.Name)

	o, err := runtime_pkg.NewScheme().DeepCopy(pod)
	if err != nil {
		return err
	}
	initializedPod := o.(*corev1.Pod)

	if !m.PodIsProtected(initializedPod) {
		// Figure out which assignments this pod matches
		scheds := m.GetPodAssignmentsScheduling(initializedPod)

		m.log.Debugf("Matched %d scheduling rule(s)", len(scheds))

		// Apply all matching assignment scheduling details to pod in order
		for _, s := range scheds {
			s.ApplyToPod(initializedPod)
		}
	}

	// Remove self from the list of pending Initializers while preserving ordering.
	if len(initializedPod.GetInitializers().Pending) == 1 {
		initializedPod.ObjectMeta.Initializers = nil
	} else {
		initializedPod.ObjectMeta.Initializers.Pending = append(initializedPod.GetInitializers().Pending[:0], initializedPod.GetInitializers().Pending[1:]...)
	}

	oldData, err := json.Marshal(pod)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(initializedPod)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, corev1.Pod{})
	if err != nil {
		return err
	}

	if !utils.IsEmptyPatch(patchBytes) {
		_, err = m.kubeClientset.CoreV1().Pods(pod.Namespace).Patch(pod.Name, types.StrategicMergePatchType, patchBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcilepod handles the business logic for NodeAssigmentGroup changes
func (m *Manager) initializePod(pod *corev1.Pod) error {
	if pod.ObjectMeta.GetInitializers() != nil &&
		pod.ObjectMeta.GetInitializers().Pending[0].Name == InitializerName {
		return m.InitializePod(pod)
	}
	return nil
}
