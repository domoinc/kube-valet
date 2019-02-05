package podassignment

import (
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/domoinc/kube-valet/pkg/queues"
	logging "github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	// InitializerName the name of the initializer we resolve
	InitializerName = "pod.initializer.kube-valet.io"
	// ProtectedLabelKey the protected label key
	ProtectedLabelKey = "pod.initializer.kube-valet.io/protected"
	// ProtectedLabelValue true
	ProtectedLabelValue = "true"
)

// Controller processes pod events and assigns based on pars and cpars
type Controller struct {
	queue    *queues.RetryingWorkQueue
	log      *logging.Logger
	podIndex cache.Indexer
	parMan   *Manager
}

// NewController creates a new Controller
func NewController(podIndex cache.Indexer, cparIndex cache.Indexer, parIndex cache.Indexer, kubeClient kubernetes.Interface, valetClient valet.Interface, threadiness int, stopChannel chan struct{}) *Controller {
	return &Controller{
		queue:    queues.NewRetryingWorkQueue("Pod", podIndex, threadiness, stopChannel),
		log:      logging.MustGetLogger("PodAssignmentController"),
		podIndex: podIndex,
		parMan:   NewManager(podIndex, cparIndex, parIndex, kubeClient),
	}
}

// Run start the podassignment.Controller
func (c *Controller) Run() {
	c.queue.Run(func(obj interface{}) error {
		pod := obj.(*corev1.Pod)
		c.log.Debugf("processing business logic for pod %s", pod.Name)
		return c.parMan.initializePod(pod)
	})
}

// OnAddPod process the pod initializer
func (c *Controller) OnAddPod(pod *corev1.Pod) {
	c.queue.AddItem(pod)
}

// OnUpdatePod if the pod has been updated process the initializer
func (c *Controller) OnUpdatePod(oldPod *corev1.Pod, newPod *corev1.Pod) {
	if (oldPod.GetResourceVersion() != newPod.GetResourceVersion()) ||
		(oldPod.GetUID() != newPod.GetUID()) {
		c.queue.AddItem(newPod)
	}
}

// OnDeletePod if the pod was deleted process the initializer
func (c *Controller) OnDeletePod(pod *corev1.Pod) {
	c.queue.AddItem(pod)
}
