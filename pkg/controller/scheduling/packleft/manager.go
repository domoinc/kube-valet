package packleft

import (
	"encoding/json"
	"fmt"
	"sort"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
)

const (
	PackLeftFinalizer string = "packleft.nag.finalizer.kube-valet.io"
)

// Manager mangaes interactions with nags and the nodes they point to
type Manager struct {
	nagIndex    cache.Indexer
	nodeIndex   cache.Indexer
	podIndex    cache.Indexer
	valetClient valet.Interface
	kubeClient  kubernetes.Interface
	log         *logging.Logger
}

// NewManager creates a new manager
func NewManager(nagIndex cache.Indexer, nodeIndex cache.Indexer, podIndex cache.Indexer, kubeClient kubernetes.Interface, valetClient valet.Interface) *Manager {
	return &Manager{
		nagIndex:    nagIndex,
		nodeIndex:   nodeIndex,
		podIndex:    podIndex,
		kubeClient:  kubeClient,
		valetClient: valetClient,
		log:         logging.MustGetLogger("PackLeftSchedulingManager"),
	}
}

// RebalanceNag rebalance nodes that are assigned to pack left assignments in a given nag
func (m *Manager) RebalanceNag(nag *assignmentsv1alpha1.NodeAssignmentGroup, metric *prometheus.GaugeVec) {
	// Ensure that the finalizer is set on the nag
	m.ensureFinalizer(nag)

	packLeftNodeGroups := m.getPackLeftNodeGroups(nag)
	m.log.Debugf("found %d node groups for nag %s with", len(packLeftNodeGroups), nag.Name)
	for assignmentName, nodes := range packLeftNodeGroups {
		if len(nodes) != 0 {
			m.log.Infof("rebalancing  %d nodes in assignment %s.%s", len(nodes), nag.Name, assignmentName)
			labelKey := getLabelKey(nag.Name)
			if assignment, ok := m.getAssignmentByName(assignmentName, nag); ok {
				m.balanceNodes(nodes, labelKey, nag, assignment, metric)
			} else {
				m.log.Warningf("Assignment %s doesn't exist in the NodeAssignmentGroup", assignmentName)
			}
		} else {
			m.log.Warningf("No nodes found for assignment %s on nag %s", assignmentName, nag.Name)
		}
	}
}

// CleanAllNodes clears all attributes for a pack left nag from all nodes
func (m *Manager) CleanAllNodes(nag *assignmentsv1alpha1.NodeAssignmentGroup) error {
	labelKey := getLabelKey(nag.Name)
	for _, obj := range m.nodeIndex.List() {
		node := obj.(*corev1.Node)
		if (!m.NodeHasPackLeftAssignment(node, nag) && m.NodeHasPackLeftAttributes(node, nag)) || !NodeCanBeBalanced(node) {
			newNode := m.unassignNode(node, labelKey)
			if err := m.patchNodeState(node, newNode); err != nil {
				return err
			}
		}
	}
	return nil
}

// CleanUnassignedNodes remove taints from nodes that are not assigned to a a packleft assignment
func (m *Manager) CleanUnassignedNodes(nag *assignmentsv1alpha1.NodeAssignmentGroup) {
	m.log.Infof("Cleaning nag %s", nag.Name)
	labelKey := getLabelKey(nag.Name)
	for _, obj := range m.nodeIndex.List() {
		node := obj.(*corev1.Node)
		if !m.NodeHasPackLeftAssignment(node, nag) && m.NodeHasPackLeftAttributes(node, nag) {
			m.log.Debugf("Node '%s' has packleft attributes for nag '%s' but is not assigned to it anymore. Clearing attributes", node.Name, nag.Name)
			newNode := m.unassignNode(node, labelKey)
			m.patchNodeState(node, newNode)
		}
	}
}

// NodeHasPackLeftAttributes checks to see if the given node has any labels or taints that have come from a given nag
func (m *Manager) NodeHasPackLeftAttributes(node *corev1.Node, nag *assignmentsv1alpha1.NodeAssignmentGroup) bool {
	lk := getLabelKey(nag.Name)
	for k := range node.GetLabels() {
		if k == lk {
			return true
		}
	}

	for _, t := range node.Spec.Taints {
		if t.Key == lk {
			return true
		}
	}

	return false
}

// NodeHasPackLeftAssignment check if node is assigned to an assignment that is set to pack left
func (m *Manager) NodeHasPackLeftAssignment(node *corev1.Node, nag *assignmentsv1alpha1.NodeAssignmentGroup) bool {
	assignments := getAllAssignments(nag)
	if na, ok := nag.GetAssignment(node); ok {
		// m.log.Debugf("Node %s has assign: %v", node.GetName(), na)
		for _, a := range assignments {
			if na == a.Name && a.SchedulingMode == assignmentsv1alpha1.NodeAssignmentSchedulingModePackLeft {
				// m.log.Debugf("Node: %s, Assign: %s, na.Name: %#v", node.GetName(), na, a)
				return true
			}
		}
	}
	return false
}

type assignmentContext struct {
	percentFull float64
	node        *corev1.Node
	assignment  *assignmentsv1alpha1.NodeAssignment
}

func newAssignmentContext(percent float64, node *corev1.Node, assignment *assignmentsv1alpha1.NodeAssignment) *assignmentContext {
	return &assignmentContext{
		percentFull: percent,
		node:        node,
		assignment:  assignment,
	}
}

func (m *Manager) balanceNodes(nodes []*corev1.Node, labelKey string, nag *assignmentsv1alpha1.NodeAssignmentGroup, assignment *assignmentsv1alpha1.NodeAssignment, metric *prometheus.GaugeVec) {
	var nodesWithPercent []*assignmentContext
	//create this first so it doesn't get created twice for every node
	podsOnNodes := m.getPodsOnNodes()
	for _, node := range nodes {
		if !NodeCanBeBalanced(node) {
			continue //filter out unschedulable nodes
		}
		var percentFull float64
		memFull := m.getNodePercentFullMemory(node, podsOnNodes)
		cpuFull := m.getNodePercentFullCPU(node, podsOnNodes)
		if memFull > cpuFull {
			percentFull = memFull
		} else {
			percentFull = cpuFull
		}
		nodesWithPercent = append(nodesWithPercent, newAssignmentContext(percentFull, node, assignment))
	}
	if len(nodesWithPercent) < 1 {
		m.log.Warningf("No schedulable nodes found. Unable to balance nodes")
		return
	}

	// sort the nodes by fullest first
	// must be deterministic when values are equal otherwise there might be state thrashing
	sort.Slice(nodesWithPercent, func(i, j int) bool {
		if nodesWithPercent[i].percentFull == nodesWithPercent[j].percentFull {
			return nodesWithPercent[i].node.Name > nodesWithPercent[j].node.Name
		}
		return nodesWithPercent[i].percentFull > nodesWithPercent[j].percentFull
	})

	// Determine the avoidBufferSize
	avoidBufferSize := 0
	if assignment.PackLeft != nil {
		if assignment.PackLeft.PercentAvoid != nil {
			// get the avoidBufferSize based on PercentAvoid. Rounding down
			avoidBufferSize = int(float32(len(nodes)) * float32(*assignment.PackLeft.PercentAvoid) / 100.0)
		}
		// if NumAvoid is larger, use it instead
		if assignment.PackLeft.NumAvoid > avoidBufferSize {
			avoidBufferSize = assignment.PackLeft.NumAvoid
		}
	}
	// avoidBufferSize cannot be < 1
	// even the smallest assignment should have at least one node set to avoid
	if avoidBufferSize < 1 {
		avoidBufferSize = 1
	}
	m.log.Debugf("attempting to leave %d nodes as 'Avoid' nodes", avoidBufferSize)

	// Determine fullPercent
	fullPercent := .8 // Default is 80%
	if assignment.PackLeft != nil && assignment.PackLeft.FullPercent != nil {
		fullPercent = float64(*assignment.PackLeft.FullPercent) / float64(100)
	}
	m.log.Debugf("nodes will be considered full at %%%v", fullPercent*100)

	denyCount := 0
	avoidCount := 0

	firstCtx := nodesWithPercent[0]
	m.log.Debugf("assigning node %s to be first full node", firstCtx.node.Name)
	firstNode := m.assignNode(firstCtx, nodeUse, labelKey, metric)
	m.patchNodeState(firstCtx.node, firstNode)

	for _, ctx := range nodesWithPercent[1:] {
		var newNode *corev1.Node
		// these calls actually save the data to kubernetes
		if ctx.percentFull > fullPercent {
			m.log.Debugf("assigned node %s to be Use", ctx.node.Name)
			newNode = m.assignNode(ctx, nodeUse, labelKey, metric)
		} else if avoidCount < avoidBufferSize {
			m.log.Debugf("assigned node %s to be Avoid", ctx.node.Name)
			newNode = m.assignNode(ctx, nodeAvoid, labelKey, metric)
			avoidCount++
		} else {
			m.log.Debugf("assigned node %s to be Deny", ctx.node.Name)
			newNode = m.assignNode(ctx, nodeDeny, labelKey, metric)
			denyCount++
		}
		m.patchNodeState(ctx.node, newNode)
	}

	if avoidBufferSize != avoidCount {
		m.log.Warningf("avoid buffer size on %s.%s is lower than specified", nag.GetName(), assignment.Name)
	}
}

type nodePackLeftState string

const (
	nodeUse      nodePackLeftState = "Use"
	nodeAvoid    nodePackLeftState = "Avoid"
	nodeDeny     nodePackLeftState = "Deny"
	nodeLabelKey                   = "nag.packleft.scheduling.kube-valet.io/%s"
)

func getLabelKey(nag string) string {
	return fmt.Sprintf(nodeLabelKey, nag)
}

func (m *Manager) patchNodeState(oldNode *corev1.Node, newNode *corev1.Node) error {

	oldData, err := json.Marshal(oldNode)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(newNode)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, corev1.Node{})
	if err != nil {
		return err
	}

	// an empty patch of "{}" has a len of 2. No need to send a zero patch
	if !utils.IsEmptyPatch(patchBytes) {
		_, err = m.kubeClient.CoreV1().Nodes().Patch(oldNode.Name, types.StrategicMergePatchType, patchBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) unassignNode(node *corev1.Node, labelKey string) *corev1.Node {
	newNode := node.DeepCopy()

	//remove the label
	delete(newNode.GetLabels(), labelKey)

	//remove the taint
	m.removeTaint(newNode, labelKey)

	return newNode
}

// Full nodes have no taint
// Filling nodes have a PreferNoSchedule taint
// Emptying nodes have a NoScheduleTaint
func (m *Manager) assignNode(ctx *assignmentContext, state nodePackLeftState, labelKey string, metric *prometheus.GaugeVec) *corev1.Node {

	metric.With(prometheus.Labels{"node_assignment": ctx.assignment.Name, "node_name": ctx.node.Name, "pack_left_state": string(state)}).Set(ctx.percentFull)

	newNode := ctx.node.DeepCopy()

	//replace label
	newNode.ObjectMeta.Labels[labelKey] = string(state)

	//remove existing taint
	m.removeTaint(newNode, labelKey)

	//add the new taint if a taint should be added
	if state != nodeUse {

		effect := corev1.TaintEffectPreferNoSchedule
		if state == nodeDeny {
			effect = corev1.TaintEffectNoSchedule
		}

		newNode.Spec.Taints = append(newNode.Spec.Taints, corev1.Taint{
			Key:    labelKey,
			Value:  string(state),
			Effect: effect,
		})
	}

	return newNode
}

func (m *Manager) removeTaint(node *corev1.Node, labelKey string) {
	var index int
	found := false
	for i, taint := range node.Spec.Taints {
		if taint.Key == labelKey {
			index = i
			found = true
			break
		}
	}
	if found {
		node.Spec.Taints = append(node.Spec.Taints[:index], node.Spec.Taints[index+1:]...)
	}
}

// get a map that contains an assignment name mapped to a list of nodes with that assignment
// only returns nodes/assignments that are pack left
func (m *Manager) getPackLeftNodeGroups(nag *assignmentsv1alpha1.NodeAssignmentGroup) map[string][]*corev1.Node {
	rtn := make(map[string][]*corev1.Node)
	targetedNodes := m.getTargetedNodes(nag)
	for _, node := range targetedNodes {
		if assignmentName, ok := nag.GetAssignment(node); ok {
			//if the assignment exists and it is a pack left assignment
			if assignment, ok := m.getAssignmentByName(assignmentName, nag); ok &&
				assignment.SchedulingMode == assignmentsv1alpha1.NodeAssignmentSchedulingModePackLeft {
				rtn[assignmentName] = append(rtn[assignmentName], node)
			}
		}
	}
	return rtn
}

func (m *Manager) getAssignmentByName(name string, nag *assignmentsv1alpha1.NodeAssignmentGroup) (*assignmentsv1alpha1.NodeAssignment, bool) {

	assignments := getAllAssignments(nag)
	for _, na := range assignments {
		if na.Name == name {
			return &na, true
		}
	}
	return nil, false
}

func (m *Manager) getTargetedNodes(assignmentGroup *assignmentsv1alpha1.NodeAssignmentGroup) []*corev1.Node {
	var rtn []*corev1.Node
	for _, obj := range m.nodeIndex.List() {
		node := obj.(*corev1.Node)
		if assignmentGroup.TargetsNode(node) {
			rtn = append(rtn, node)
		}
	}
	return rtn
}

func (m *Manager) getNodePercentFullMemory(node *corev1.Node, podsOnNodes map[string][]*corev1.Pod) float64 {
	var bytesRequested int64
	for _, pod := range podsOnNodes[node.Name] {
		// Completed pods don't count against schedulable capacity
		if pod.Status.Phase != corev1.PodSucceeded {
			for _, container := range pod.Spec.Containers {
				bytesRequested += container.Resources.Requests.Memory().Value()
			}
		}
	}

	return float64(bytesRequested) / float64(node.Status.Allocatable.Memory().Value())
}

func (m *Manager) getNodePercentFullCPU(node *corev1.Node, podsOnNodes map[string][]*corev1.Pod) float64 {
	var minutesRequested int64
	for _, pod := range podsOnNodes[node.Name] {
		// Completed pods don't count against schedulable capacity
		if pod.Status.Phase != corev1.PodSucceeded {
			for _, container := range pod.Spec.Containers {
				minutesRequested += container.Resources.Requests.Cpu().ScaledValue(-3)
			}
		}
	}

	return float64(minutesRequested) / float64(node.Status.Allocatable.Cpu().ScaledValue(-3))
}

// no need to optimize by only looking for one node because all pods have to be looked at anyway
// might want to memozie/cache this somehow
func (m *Manager) getPodsOnNodes() map[string][]*corev1.Pod {
	rtn := make(map[string][]*corev1.Pod)
	for _, obj := range m.podIndex.List() {
		pod := obj.(*corev1.Pod)
		rtn[pod.Spec.NodeName] = append(rtn[pod.Spec.NodeName], pod)
	}
	return rtn
}

func (m *Manager) getPackLeftNodeAssignment(nag *assignmentsv1alpha1.NodeAssignmentGroup) []*assignmentsv1alpha1.NodeAssignment {
	var rtn []*assignmentsv1alpha1.NodeAssignment

	assignments := getAllAssignments(nag)
	for _, na := range assignments {
		if na.SchedulingMode == assignmentsv1alpha1.NodeAssignmentSchedulingModePackLeft {
			rtn = append(rtn, na.DeepCopy())
		}
	}

	return rtn
}

func getAllAssignments(nag *assignmentsv1alpha1.NodeAssignmentGroup) []assignmentsv1alpha1.NodeAssignment {
	if nag.Spec.DefaultAssignment != nil {
		return append(nag.Spec.Assignments, *nag.Spec.DefaultAssignment)
	}
	return nag.Spec.Assignments
}

func (m *Manager) ensureFinalizer(nag *assignmentsv1alpha1.NodeAssignmentGroup) error {
	// Only add finalizer if it's not already present
	for _, f := range nag.GetFinalizers() {
		if f == PackLeftFinalizer {
			return nil
		}
	}

	// Finalizer not already set. Add it
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := m.valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Get(nag.GetName(), metav1.GetOptions{})
		if getErr != nil {
			m.log.Errorf("Failed to get latest version of nag: %v", getErr)
		}

		// Add Finalizer
		result.SetFinalizers(append(result.GetFinalizers(), PackLeftFinalizer))

		_, updateErr := m.valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Update(result)
		return updateErr
	})

	if retryErr != nil {
		m.log.Errorf("Update failed: %+v", retryErr)
		return retryErr
	}

	m.log.Debug("Added PackLeft finalizer")
	return nil
}

func (m *Manager) RemoveFinalizer(nag *assignmentsv1alpha1.NodeAssignmentGroup) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := m.valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Get(nag.GetName(), metav1.GetOptions{})
		if getErr != nil {
			m.log.Errorf("Failed to get latest version of nag: %v", getErr)
		}

		// Create filtered finalizers list, remove completed finalizer
		newFinalizers := utils.FilterValues(result.GetFinalizers(), func(v string) bool {
			if PackLeftFinalizer == v {
				return false
			}
			return true
		})

		// Set new list of finalizers on obj and update
		result.SetFinalizers(newFinalizers)

		_, updateErr := m.valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Update(result)
		return updateErr
	})

	if retryErr != nil {
		m.log.Errorf("Update failed: %+v", retryErr)
	}

	m.log.Debug("Removed NAG finalizer")
	return retryErr
}

func NodeCanBeBalanced(node *corev1.Node) bool {
	isReady := false
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			isReady = cond.Status == corev1.ConditionTrue
			break
		}
	}

	return !node.Spec.Unschedulable && isReady
}
