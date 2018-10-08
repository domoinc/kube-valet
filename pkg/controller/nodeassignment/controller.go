package nodeassignment

import (
	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valetclient "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/domoinc/kube-valet/pkg/queues"
	"github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

//Controller listens for changes to NodeAssignmentGroups and Nodes to reset allocation of nodes
type Controller struct {
	queue    *queues.RetryingWorkQueue
	log      *logging.Logger
	nagIndex cache.Indexer
	nagm     *Manager
}

//NewController creates a new Controller
func NewController(nagIndex cache.Indexer, nodeIndex cache.Indexer, kubeClientset *kubernetes.Clientset, valetClientset *valetclient.Clientset, threadiness int, stopChannel chan struct{}) *Controller {
	return &Controller{
		queue:    queues.NewRetryingWorkQueue("NodeAssignmentGroup", nagIndex, threadiness, stopChannel),
		log:      logging.MustGetLogger("NodeAssignmentController"),
		nagIndex: nagIndex,
		nagm:     NewManager(kubeClientset, valetClientset),
	}
}

// Run starts the nodeassignment.Controller
func (c *Controller) Run() {
	c.queue.Run(func(obj interface{}) error {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		c.log.Debugf("processing business logic for nag %s", nag.Name)
		err := c.nagm.ReconcileNag(nag)
		return err
	})
}

// OnAddNode queue all nags for processing.
func (c *Controller) OnAddNode(node *corev1.Node) {
	c.log.Debugf("NodeAssignment: Node %s added. Requeueing all Nags", node.GetName())
	c.queueAllNags()
}

// OnUpdateNode recalculates all nags if targeting attributes have changed
func (c *Controller) OnUpdateNode(oldNode *corev1.Node, newNode *corev1.Node) {
	// Only trigger on changes to targetable attributes. This avoids excessive churn due to node status updates
	if utils.NodeTargetingHasChanged(oldNode, newNode) {
		c.log.Debugf("NodeAssignment: Node %s has updated targetable attributes. Requeueing all Nags", oldNode.GetName())
		c.queueAllNags()
	}
}

// OnDeleteNode when a node is deleted process the applicable nag
func (c *Controller) OnDeleteNode(node *corev1.Node) {
	nags := c.getNodeNags(node)
	for _, nag := range nags {
		c.OnAddNag(nag)
	}
}

// OnAddNag process and added nag
func (c *Controller) OnAddNag(nag *assignmentsv1alpha1.NodeAssignmentGroup) {
	c.log.Debugf("Adding nag %s to queue", nag.Name)
	c.queue.AddItem(nag)
}

//OnUpdateNag if the nag has changed process it
func (c *Controller) OnUpdateNag(oldNag *assignmentsv1alpha1.NodeAssignmentGroup, newNag *assignmentsv1alpha1.NodeAssignmentGroup) {
	c.log.Debug("Update Nag", oldNag.GetResourceVersion(), newNag.GetResourceVersion())
	// Only add to the queue if there is an actual change
	if oldNag.GetResourceVersion() != newNag.GetResourceVersion() || (oldNag.GetUID() != newNag.GetUID()) {
		c.queue.AddItem(newNag)
	} else {
		c.log.Debug("Ignoring nag update (nothing has changed)")
	}
}

//OnDeleteNag if a nag was deleted process it
func (c *Controller) OnDeleteNag(nag *assignmentsv1alpha1.NodeAssignmentGroup) {
	c.log.Debugf("Adding nag %s to queue", nag.Name)
	c.queue.AddItem(nag)
}

func (c *Controller) getNodeNags(node *corev1.Node) []*assignmentsv1alpha1.NodeAssignmentGroup {
	var rtn []*assignmentsv1alpha1.NodeAssignmentGroup
	for _, obj := range c.nagIndex.List() {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		if nag.TargetsNode(node) {
			rtn = append(rtn, nag.DeepCopy())
		}
	}
	return rtn
}

// queueAllNags Will queue all nags for reconciliation
func (c *Controller) queueAllNags() {
	for _, obj := range c.nagIndex.List() {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		c.queue.AddItem(nag)
	}
}
