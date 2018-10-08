package nodeassignment

import (
	"encoding/json"

	"github.com/domoinc/kube-valet/pkg/utils"
	logging "github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
)

type WriterContext struct {
	Nag                 *assignmentsv1alpha1.NodeAssignmentGroup
	KnownAssignments    map[string]struct{}
	TargetedNodes       []*corev1.Node
	UntargetedNodes     []*corev1.Node
	CurrentAssignments  map[string]int
	AssignmentChanges   map[string]int
	UnassignedNodeNames map[string]struct{}
	kubeClientset       *kubernetes.Clientset
	log                 *logging.Logger
}

func NewWriterContext(kubeClientSet *kubernetes.Clientset, nag *assignmentsv1alpha1.NodeAssignmentGroup) *WriterContext {
	wc := &WriterContext{
		kubeClientset:       kubeClientSet,
		Nag:                 nag,
		UnassignedNodeNames: make(map[string]struct{}),
		log:                 logging.MustGetLogger("NodeAssignmentModel"),
	}
	// initializes and populates all other struct fields
	wc.Update()
	return wc
}

func (wc *WriterContext) Update() {
	wc.log.Debug("updating")
	wc.updateKnownAssignments()
	wc.updateNodeSets()
	wc.updateCurrentAssignments()
	wc.updateAssignmentChanges()
}

// updateKnownAssignments Generates a map of all known assignments in a group and sets the struct field
func (wc *WriterContext) updateKnownAssignments() map[string]struct{} {
	wc.KnownAssignments = make(map[string]struct{})
	for _, a := range wc.Nag.Spec.Assignments {
		wc.KnownAssignments[a.Name] = struct{}{}
	}
	// Default is asignment is is "known" as well
	if wc.Nag.Spec.DefaultAssignment != nil {
		wc.KnownAssignments[wc.Nag.Spec.DefaultAssignment.Name] = struct{}{}
	}
	return wc.KnownAssignments
}

// updateNodeSets updates a slice of pointers to copied node objects
func (wc *WriterContext) updateNodeSets() error {
	// Get nodes from api
	nodes, err := wc.kubeClientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		wc.log.Fatal(err)
		return err
	}

	for _, node := range nodes.Items {
		// Always unassign nodes assigned to assignments that are no longer in the group
		if ca, ok := wc.Nag.GetAssignment(&node); ok {
			if _, ok := wc.KnownAssignments[ca]; !ok {
				wc.log.Debugf("%s is part of an unknown assignment: %s. Unassigning", node.ObjectMeta.Name, ca)
				// Unassign in memory only. Makes any reassignments atomic
				wc.Nag.Unassign(&node)
				// Keep track of all nodes that have been unassigned. If they are not reassigned by the end of reconciliation
				// then they will need to actually be unassigned in the api
				wc.UnassignedNodeNames[node.ObjectMeta.Name] = struct{}{}
			}
		}

		if wc.Nag.TargetsNode(&node) {
			wc.log.Debug("targeting node", node.GetObjectMeta().GetName())
			wc.TargetedNodes = append(wc.TargetedNodes, node.DeepCopy())
		} else {
			wc.log.Debug("not targeting node", node.GetObjectMeta().GetName())
			if curAssign, ok := wc.Nag.GetAssignment(&node); ok {
				wc.log.Debugf("%s is no longer targeted by %s but has an assignment. Unassigning", node.ObjectMeta.Name, curAssign)
				if err := wc.UpdateNodeAssignment(&node, nil); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (wc *WriterContext) updateCurrentAssignments() {
	wc.CurrentAssignments = make(map[string]int)
	// Generate map of current assigments and their satisfactions
	for _, node := range wc.TargetedNodes {
		if a, ok := wc.Nag.GetAssignment(node); ok {
			if wc.Nag.Spec.DefaultAssignment == nil ||
				(wc.Nag.Spec.DefaultAssignment != nil && a != wc.Nag.Spec.DefaultAssignment.Name) {
				wc.CurrentAssignments[a]++
			}
		}
	}
	wc.log.Debugf("Current Assignments: %+v", wc.CurrentAssignments)
}

func (wc *WriterContext) assignNode(node *corev1.Node) (bool, error) {
	wc.log.Debug("assigning node", node.GetObjectMeta().GetName())
	for _, a := range wc.Nag.Spec.Assignments {
		// If assigned to assignment with positive delta
		if d, ok := wc.AssignmentChanges[a.Name]; ok && d > 0 {
			// Needs more nodes
			// Assign and break out of loop
			if err := wc.UpdateNodeAssignment(node, &a); err != nil {
				return false, err
			}
			// One fewer node is required now
			wc.AssignmentChanges[a.Name]--

			// Cleanup
			if wc.AssignmentChanges[a.Name] == 0 {
				delete(wc.AssignmentChanges, a.Name)
			}

			// Next node
			return true, nil
		}
	}
	return false, nil
}

func (wc *WriterContext) updateAssignmentChanges() {
	wc.AssignmentChanges = make(map[string]int)
	// Calculate any changes that are required
	for _, a := range wc.Nag.Spec.Assignments {
		// get the number of desired nodes based on PercentDesired. Rounding down
		desired := int(float32(len(wc.TargetedNodes)) * float32(a.PercentDesired) / 100.0)
		// if NumDesired is given, and is larger, use it instead
		if a.NumDesired > desired {
			desired = a.NumDesired
		}
		curNum, ok := wc.CurrentAssignments[a.Name]
		if !ok {
			curNum = 0
		}
		d := desired - curNum

		if d != 0 {
			wc.AssignmentChanges[a.Name] = d
		}
	}
	wc.log.Debugf("Assignment Changes: %+v", wc.AssignmentChanges)
}

// UpdateNodeAssignment uses the NodeAssignmentController's clients to do api updates
func (wc *WriterContext) UpdateNodeAssignment(node *corev1.Node, na *assignmentsv1alpha1.NodeAssignment) error {
	o, err := runtime.NewScheme().DeepCopy(node)
	if err != nil {
		return err
	}
	assignedNode := o.(*corev1.Node)

	// Add or remove labels/taints
	if na != nil {
		wc.log.Debug("Assigning node:", assignedNode.GetName(), "to", na.Name)
		wc.Nag.Assign(assignedNode, na)
	} else {
		wc.log.Debug("Unassigning from group")
		wc.Nag.Unassign(assignedNode)
	}

	oldData, err := json.Marshal(node)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(assignedNode)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, corev1.Node{})
	if err != nil {
		return err
	}

	// an empty patch of "{}" has a len of 2. No need to send a zero patch
	if !utils.IsEmptyPatch(patchBytes) {
		_, err = wc.kubeClientset.CoreV1().Nodes().Patch(node.Name, types.StrategicMergePatchType, patchBytes)
		if err != nil {
			return err
		}
	}

	if _, ok := wc.UnassignedNodeNames[assignedNode.ObjectMeta.Name]; ok {
		delete(wc.UnassignedNodeNames, assignedNode.ObjectMeta.Name)
	}

	return nil
}

func (wc *WriterContext) Reconcile() error {
	wc.log.Info("Reconciling Assignments for NAG:", wc.Nag.ObjectMeta.Name)

	// new empty status
	// s := assignmentsv1alpha1.NodeAssignmentGroupStatus{}

	// Loop through targeted nodes and update assignments
	for _, node := range wc.TargetedNodes {
		if ca, ok := wc.Nag.GetAssignment(node); ok {
			var unassign bool
			if d, ok := wc.AssignmentChanges[ca]; ok && d < 0 {
				// If assigned to assignment with negative delta, unassign
				wc.log.Debugf("%s should no longer be assigned to %s", node.ObjectMeta.Name, ca)
				unassign = true
			} else if wc.Nag.Spec.DefaultAssignment != nil && ca == wc.Nag.Spec.DefaultAssignment.Name {
				// if the assignment is the default assignment then it can be reassigned
				wc.log.Debugf("%s is part of the default assignment and can be reassigned", node.ObjectMeta.Name)
				unassign = true
			} else {
				// assigned to assignment that doesn't requires changes
				wc.log.Debugf("%s will stay assigned to %s", node.ObjectMeta.Name, ca)
				continue
			}

			if unassign {
				// Unassign in memory. Allows atomic reassigns
				wc.Nag.Unassign(node)

				// Add to Unassign cleanup map
				wc.UnassignedNodeNames[node.ObjectMeta.Name] = struct{}{}

				// One fewer unassignments needed
				wc.AssignmentChanges[ca]++

				// Cleanup
				if wc.AssignmentChanges[ca] == 0 {
					delete(wc.AssignmentChanges, ca)
				}
			}
		} else {
			wc.log.Debugf("%s is not currently assigned", node.ObjectMeta.Name)
		}

		if ok, err := wc.assignNode(node); ok {
			if err != nil {
				wc.log.Error("Error assigning node", node)
			}
			// Move on to next node
			continue
		}

		// No more assignment changes required
		// if there is a default not currently applied then apply it
		if _, ok := wc.Nag.GetAssignment(node); !ok && wc.Nag.Spec.DefaultAssignment != nil {
			wc.log.Debugf("Assigning %s to default assignment", node.ObjectMeta.Name)
			if err := wc.UpdateNodeAssignment(node, wc.Nag.Spec.DefaultAssignment); err != nil {
				return err
			}
		}
	}

	// Unassign any nodes that were not atomically reassigned
	for nodeName := range wc.UnassignedNodeNames {
		wc.log.Debug("Updating", nodeName, "to be unassigned")
		if err := wc.UnassignNodeByName(nodeName); err != nil {
			wc.log.Error("Error unassigning Node", err)
		}
	}

	return nil
}

// UnassignNodeByName get's the latest version of a node from the api and unassigns it
func (wc *WriterContext) UnassignNodeByName(name string) error {
	wc.log.Debugf("Unassigning node %s", name)

	node, err := wc.kubeClientset.CoreV1().Nodes().Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return wc.UpdateNodeAssignment(node, nil)
}

// UnassignAllNodes cleans all nodes of assignment labels/taints
// This will effect any nodes that have label or taint
// for the group, Even if they no longer match the targetLabels
// Ensuring that deleting a group always removes ALL traces of the group from nodes
func (wc *WriterContext) UnassignAllNodes() error {
	wc.log.Debugf("Unassigning all Assignments from %s", wc.Nag.ObjectMeta.Name)

	nodes, err := wc.kubeClientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		wc.UpdateNodeAssignment(&node, nil)
	}

	return nil
}
