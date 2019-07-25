package packleft

import (
	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/domoinc/kube-valet/pkg/metrics"
	"github.com/domoinc/kube-valet/pkg/queues"
	"github.com/domoinc/kube-valet/pkg/utils"
)

// Controller manages events for pods, nodes, and nags and rebalances pack left strategies on the nag
type Controller struct {
	queue       *queues.RetryingWorkQueue
	kubeClient  kubernetes.Interface
	valetClient valet.Interface
	plm         *Manager
	nagIndex    cache.Indexer
	nodeIndex   cache.Indexer
	log         *logging.Logger
	registry    *metrics.Registry
}

// NewController creates a new packleft.Controller
func NewController(nagIndex cache.Indexer, nodeIndex cache.Indexer, podIndex cache.Indexer, kubeClient kubernetes.Interface, valetClient valet.Interface, threadiness int, stopChannel chan struct{}) *Controller {
	return &Controller{
		queue:     queues.NewRetryingWorkQueue("NodeAssignmentGroup", nagIndex, threadiness, stopChannel),
		plm:       NewManager(nagIndex, nodeIndex, podIndex, kubeClient, valetClient),
		nagIndex:  nagIndex,
		nodeIndex: nodeIndex,
		log:       logging.MustGetLogger("PackLeftSchedulingController"),
		registry:  metrics.NewRegistry(),
	}
}

// Run starts the controller
func (plc *Controller) Run() {
	plc.queue.Run(func(obj interface{}) error {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		plc.log.Debugf("processing business logic for nag %s", nag.Name)
		//reset nag metrics
		metric := plc.registry.GetPackLeftPercentFull(nag.Name)
		metric.Reset()
		if nag.GetDeletionTimestamp() != nil {
			if err := plc.plm.CleanAllNodes(nag); err != nil {
				return err
			}
			if err := plc.plm.RemoveFinalizer(nag); err != nil {
				return err
			}
		} else {
			plc.plm.RebalanceNag(nag, metric)
			//clean up nodes that are no longer part of the nag but have labels
			plc.plm.CleanUnassignedNodes(nag)
		}
		return nil
	})
}

// OnAddNode when a node is added, queue a process of all nags
func (plc *Controller) OnAddNode(node *corev1.Node) {
	plc.log.Debugf("Packleft: Node %s added or workload changed. Requeueing all Nags", node.GetName())
	plc.queueAllNags()
}

// OnUpdateNode when a node is updated rebalance all the nags that point to it
func (plc *Controller) OnUpdateNode(oldNode *corev1.Node, newNode *corev1.Node) {
	if utils.NodeTargetingHasChanged(oldNode, newNode) || (NodeCanBeBalanced(oldNode) != NodeCanBeBalanced(newNode)) {
		plc.log.Debugf("PackLeft: Node %s has updated targetable attributes. Requeueing all Nags", oldNode.GetName())
		plc.queueAllNags()
	}
}

// ProcessPackLeftNagsForNode takes a node and queues a process for all nags that target it.
func (plc *Controller) ProcessPackLeftNagsForNode(node *corev1.Node) {
	// get the nags that apply to this node
	nags := plc.getNodeAssignmentGroupsWithPackLeft(node)
	for _, nag := range nags {
		plc.OnAddNag(nag)
	}
}

// OnDeleteNode when a node is deleted rebalance the nags that used to point to it
func (plc *Controller) OnDeleteNode(node *corev1.Node) {
	plc.ProcessPackLeftNagsForNode(node)
}

// OnAddNag when a nag is added rebalance it
func (plc *Controller) OnAddNag(nag *assignmentsv1alpha1.NodeAssignmentGroup) {
	plc.log.Debugf("adding nag %s to queue", nag.Name)
	plc.queue.AddItem(nag)
}

// OnUpdateNag when a nag is updated rebalance it
func (plc *Controller) OnUpdateNag(oldNag *assignmentsv1alpha1.NodeAssignmentGroup, newNag *assignmentsv1alpha1.NodeAssignmentGroup) {
	plc.log.Debugf("adding nag %s to queue", newNag.Name)
	plc.queue.AddItem(newNag)
}

// OnDeleteNag when a nag is deleted clean up all the nodes to make sure they don't have taints from this nag
// clean up the finalizer
func (plc *Controller) OnDeleteNag(nag *assignmentsv1alpha1.NodeAssignmentGroup) {
	plc.log.Debugf("adding nag %s to queue", nag.Name)
	plc.queue.AddItem(nag)
}

// ProcessPodNags finds all the nags associated with a pod and queues a process of them.
func (plc *Controller) ProcessPodNags(pod *corev1.Pod) {
	node := plc.getNodeHostingPod(pod)
	if node != nil {
		plc.ProcessPackLeftNagsForNode(node)
	}
}

// OnAddPod when a pod is added rebalance the nag that points to the node the pod is running on
// If the pod is brand new then it may not be on a node yet and the process must be triggered on pod update
func (plc *Controller) OnAddPod(pod *corev1.Pod) {
	plc.ProcessPodNags(pod)
}

// OnUpdatePod processes pod updates for PackLeft. The rebalance is only triggered if the NodeName changes
// this typically happens when a pod is first scheduled onto a node
func (plc *Controller) OnUpdatePod(oldPod *corev1.Pod, newPod *corev1.Pod) {
	// pods don't move nodes, but they do go from no node to a node
	if oldPod.Spec.NodeName != newPod.Spec.NodeName {
		node := plc.getNodeHostingPod(newPod)
		if node != nil {
			plc.OnAddNode(node)
		}
	}
}

// OnDeletePod when a pod is deleted rebalance tha nag that points to the node the pod is running on
func (plc *Controller) OnDeletePod(pod *corev1.Pod) {
	plc.ProcessPodNags(pod)
}

func (plc *Controller) getNodeHostingPod(pod *corev1.Pod) *corev1.Node {
	for _, obj := range plc.nodeIndex.List() {
		node := obj.(*corev1.Node)
		if node.Name == pod.Spec.NodeName {
			return node
		}
	}
	return nil
}

func (plc *Controller) getNodeAssignmentGroupsWithPackLeft(node *corev1.Node) []*assignmentsv1alpha1.NodeAssignmentGroup {
	var rtn []*assignmentsv1alpha1.NodeAssignmentGroup

	for _, obj := range plc.nagIndex.List() {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		if nag.TargetsNode(node) && plc.plm.NodeHasPackLeftAssignment(node, nag) {
			rtn = append(rtn, nag)
		}
	}
	return rtn
}

// queueAllNags Will queue all nags for reconciliation
func (plc *Controller) queueAllNags() {
	for _, obj := range plc.nagIndex.List() {
		nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
		plc.queue.AddItem(nag)
	}
}
