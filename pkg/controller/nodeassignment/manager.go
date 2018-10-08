package nodeassignment

import (
	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valetclient "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	nagFinalizer string = "nag.finalizer.kube-valet.io"
)

type Manager struct {
	kubeClientset     *kubernetes.Clientset
	valetClientset *valetclient.Clientset
	log               *logging.Logger
}

func NewManager(kubeClientset *kubernetes.Clientset, valetClientset *valetclient.Clientset) *Manager {
	return &Manager{
		kubeClientset:     kubeClientset,
		valetClientset: valetClientset,
		log:               logging.MustGetLogger("NodeAssignmentManager"),
	}
}

// ReconcileNag handles the business logic for NodeAssigmentGroup changes
func (m *Manager) ReconcileNag(nag *assignmentsv1alpha1.NodeAssignmentGroup) error {
	// Note that you also have to check the uid if you have a local controlled resource, which
	// is dependent on the actual instance, to detect that a NodeAssignmentGroup was recreated with the same name
	m.log.Debugf("Sync/Add/Update for NodeAssignmentGroup %s\n", nag.GetName())

	// Create a new NagController
	nagWc := NewWriterContext(m.kubeClientset, nag)

	if nag.GetDeletionTimestamp() == nil {
		m.log.Debug("Handling NAG Add/Update")
		// try to add the finalizer
		added, err := m.AddFinalizer(nag)
		if err != nil {
			return err
		}

		// If a finalizer was added to the group then the update event will do the reconciling. No need to do it twice
		if !added {
			if err := nagWc.Reconcile(); err != nil {
				return err
			}
		}
	} else {
		m.log.Debug("Handling NAG Finalizer")
		// Delete timestamp exists. Triggger delete actions
		nagWc.UnassignAllNodes()
		m.RemoveFinalizer(nag)
	}
	return nil
}

func (m *Manager) AddFinalizer(nag *assignmentsv1alpha1.NodeAssignmentGroup) (bool, error) {
	// Only add finalizer if it's not already present
	for _, f := range nag.GetFinalizers() {
		if f == nagFinalizer {
			return false, nil
		}
	}

	// Finalizer not already set. Add it
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := m.valetClientset.AssignmentsV1alpha1().NodeAssignmentGroups().Get(nag.GetName(), metav1.GetOptions{})
		if getErr != nil {
			m.log.Errorf("Failed to get latest version of nag: %v", getErr)
		}

		// Add Finalizer
		result.SetFinalizers(append(result.GetFinalizers(), nagFinalizer))

		_, updateErr := m.valetClientset.AssignmentsV1alpha1().NodeAssignmentGroups().Update(result)
		return updateErr
	})

	if retryErr != nil {
		m.log.Errorf("Update failed: %+v", retryErr)
		return false, retryErr
	}

	m.log.Debug("Added NAG finalizer")
	return true, nil
}

func (m *Manager) RemoveFinalizer(nag *assignmentsv1alpha1.NodeAssignmentGroup) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := m.valetClientset.AssignmentsV1alpha1().NodeAssignmentGroups().Get(nag.GetName(), metav1.GetOptions{})
		if getErr != nil {
			m.log.Errorf("Failed to get latest version of nag: %v", getErr)
		}

		// Create filtered finalizers list, remove completed finalizer
		newFinalizers := utils.FilterValues(result.GetFinalizers(), func(v string) bool {
			if nagFinalizer == v {
				return false
			}
			return true
		})

		// Set new list of finalizers on obj and update
		result.SetFinalizers(newFinalizers)

		_, updateErr := m.valetClientset.AssignmentsV1alpha1().NodeAssignmentGroups().Update(result)
		return updateErr
	})

	if retryErr != nil {
		m.log.Errorf("Update failed: %+v", retryErr)
	}

	m.log.Debug("Removed NAG finalizer")
	return retryErr
}
