package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ghodss/yaml"

	"gopkg.in/alecthomas/kingpin.v2"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
)

const (
	groupCreateCmdHelp = `Create a NodeAssignmentGroup

Examples:
  # Target: disk=ssd nodes
  # Ensure all nodes are labeled and tainted for the 'fast' assignment
  valetctl group create io -t disk=ssd fast:DEFAULT:LabelAndTaint

  # Target: nodetype=general-purpose nodes
  # Ensure there is always one 'group-a' labeled node and one 'group-b' labeled and tainted node, no default
  valetctl group create targeted -t nodetype=general-purpose group-a:1 group-b:1:LabelAndTaint

  # Target: All nodes
  # Ensure that there is always one 'assign1', 10% 'assign2', and the rest are 'assign3'
  valetctl group create mygroup assign1:1 assign2:10% assig3:DEFAULT
  
  # Target: All nodes
  # Ensure that 25% of nodes are labeled and tainted for 'assign1' and the rest are for 'defAssign'
  valetctl group create tainted assign1:25%:LabelAndTaint defAssign:DEFAULT:LabelAndTaint
`

	assignmentCreateCmdHelp = `Create ClusterPodAssignmentRules or PodAssignmentRules

Affinity Examples:

  # Target: io-load=heavy pods in the default namespace
  # Pods will be given a prefered affinity to 'io/*' node assignment members. Implies toleration
  valetctl assignment create io-heavy prefer -n default -t io-load=heavy -A io
  
  # Target: io-load=xheavy pods in all namespaces
  # Pods will be given a required affinity to 'io/fast' node assignment members. Implies toleration
  valetctl assignment create io-xheavy require -t io-load=xheavy -A io/fast

  # Target: job=misc pods in all namespaces
  # Pods will use nodeAffinity to require jobhost=mysql or jobhost=etl nodes
  valetctl assignment create misc-jobs require -t job=misc -a jobhost=mysql,etl

NodeSelector Examples:
  # Target: sensitive=true pods in all namespaces
  # Pods will use a nodeSelector to require volatile=false labeled nodes
  valetctl assignment create sensitive-no-volatile nodes -t sensitive=true -S volatile=false

PodAffinity/AntiAffinity Examples:
  # Target: tier=db,dbname=db1 pods in the default namespace
  # Pods will be given an avoid anti-affinity to avoid running on the same node of other pods that match the rule
  valetctl assignment create db1-avoid avoid-others -n default -t tier=db,dbname=db1

  # Target: tier=db,dbname=db2 pods in the default namespace
  # Pods will be given an avoid anti-affinity to avoid running on the same node of other pods that match the rule
  valetctl assignment create db2-require require-others -n default -t tier=db,dbname=db2

  # Target: tier=db,dbname=db-critical pods in all namespaces
  # Pods will be given an deny anti-affinity to never run on a node in same zone of other pods that match the rule
  valetctl assignment create db-critical-deny deny-others -t tier=db,dbname=db-critical -F cloud-zone
`
)

var (
	app        = kingpin.New("valetctl", "Kube-valet command-line control")
	kubeconfig = app.Flag("kubeconfig", "Path to kubeconfig").Short('c').String()
	dryRun     = app.Flag("dry-run", "Output objects without submitting them to Kubernetes").Short('N').Bool()
	output     = app.Flag("output", "Output format. Options: yaml, json").Short('o').Default("yaml").Enum("yaml", "json")

	groupCmd = app.Command("group", "Work with NodeAssignmentGroups")

	groupCreateCmd             = groupCmd.Command("create", groupCreateCmdHelp)
	groupCreateCmdTargetLabels = groupCreateCmd.Flag("target-labels", "Label selector for nodes").Short('t').String()
	groupCreateCmdName         = groupCreateCmd.Arg("name", "Group name").Required().String()
	groupCreateCmdAssignments  = groupCreateCmd.Arg("assignments", "Assignment pairs. NAME:NUM:MODE. NUM can be a number, percent, or `DEFAULT`. NUM is optional. If no NUM is given, DEFAULT is assumed. MODE can be 'LabelOnly' or 'LabelAndTaint'. MODE is optional. If no MODE is given, labelOnly is assumed").Required().Strings()

	groupReportCmd = groupCmd.Command("report", "Generate NodeAssignmentGroup reports.")

	groupReportNagsCmd      = groupReportCmd.Command("nags", "Generate report based on NodeAssignmentGroup")
	groupReportNagsCmdNames = groupReportNagsCmd.Arg("targets", "Target NodeAssignmentGroup Names").Strings()

	groupReportNodesCmd      = groupReportCmd.Command("nodes", "Generate report based on Node")
	groupReportNodesCmdNames = groupReportNodesCmd.Arg("targets", "Target Node Names").Strings()

	assignmentCmd = app.Command("assignment", "Work with ClusterPodAssignmentRules and PodAssignmentRules")

	assignmentCreateCmd             = assignmentCmd.Command("create", assignmentCreateCmdHelp)
	assignmentCreateCmdNamespace    = app.Flag("namespace", "Create a PodAssignmentRule in the given namespace").Short('n').String()
	assignmentCreateCmdTargetLabels = assignmentCreateCmd.Flag("target-labels", "Label selector for pods").Short('t').String()
	assignmentCreateCmdNodeSelector = assignmentCreateCmd.Flag("node-selector", "NodeSelector labels").Short('S').String()
	assignmentCreateCmdNodeAffinity = assignmentCreateCmd.Flag("node-affinity", "Node Affinity labels. Supports key=value1,value2,etc...").Short('a').String()
	assignmentCreateCmdAssignment   = assignmentCreateCmd.Flag("assignment", "NodeAssignmentGroup Name/Assignment").Short('A').String()
	assignmentCreateCmdTopologyKey  = assignmentCreateCmd.Flag("fault-key", "Fault key for avoid-others and deny-thers anti-affinity").Short('F').Default("kubernetes.io/hostname").String()
	assignmentCreateCmdName         = assignmentCreateCmd.Arg("name", "Assignment name").Required().String()
	assignmentCreateCmdMode         = assignmentCreateCmd.Arg("mode", "Rule Mode").Required().Enum("prefer", "require", "nodes", "prefer-others", "require-others", "avoid-others", "deny-others")

	assignmentReportCmd = assignmentCmd.Command("report", "Report on ClusterPodAssignmentRules and PodAssignmentRules")

	restConfig  *rest.Config
	kubeClient  *kubernetes.Clientset
	valetClient *valet.Clientset
)

func main() {
	// Enable short help flag
	app.HelpFlag.Short('h')

	if !*dryRun {
		configureClients()
	}

	// Parse Flags
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))
	switch cmd {
	case groupCreateCmd.FullCommand():
		groupCreate()
	case groupReportNagsCmd.FullCommand():
		if *dryRun {
			app.Fatalf("Report cannot be run in dry-run mode")
		}
		groupReportByNag()
	case groupReportNodesCmd.FullCommand():
		if *dryRun {
			app.Fatalf("Report cannot be run in dry-run mode")
		}
		groupReportByNode()
	case assignmentCreateCmd.FullCommand():
		// if *assignmentCreateCmdNodeSelector == "" && *assignmentCreateCmdAssignment == "" && *assignmentCreateCmdNodeAffinity == "" {
		// 	app.FatalUsage("A node-selector, nodeaffinity, or assignment flag must be provided")
		// }
		assignmentCreate()
	}

}

func configureClients() (*kubernetes.Clientset, *valet.Clientset) {
	// Get kubeconfig
	restConfig = getConfig()

	// create the kubernetes client
	var err error
	kubeClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		app.Fatalf("Error creating Kubernetes client: %s\n", err)
	}

	// create the valet client
	valetClient, err = valet.NewForConfig(restConfig)
	if err != nil {
		app.Fatalf("Error creating kube-valet client: %s\n", err)
	}

	return kubeClient, valetClient
}

func getConfig() *rest.Config {
	// If there is no config flag, go through fallbacks
	if *kubeconfig == "" {
		envConfig := os.Getenv("KUBECONFIG")
		home := homedir.HomeDir()
		if envConfig != "" { // Check for a path in env
			kubeconfig = &envConfig
		} else if home != "" { // use the config from the users home dir
			homeConfig := filepath.Join(home, ".kube", "config")
			kubeconfig = &homeConfig
		}
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		app.Fatalf("Error getting Kubernetes client config at '%s': %s\n", *kubeconfig, err)
	}
	return config
}

func groupCreate() {
	targetLabels, err := labels.ConvertSelectorToLabelsMap(*groupCreateCmdTargetLabels)
	if err != nil {
		app.FatalUsage("Error parsing targetLabels: %s\n", err)
	}

	nag := &assignmentsv1alpha1.NodeAssignmentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: *groupCreateCmdName,
		},
		Spec: assignmentsv1alpha1.NodeAssignmentGroupSpec{
			TargetLabels: targetLabels,
		},
	}

	for _, assign := range *groupCreateCmdAssignments {
		parts := strings.SplitN(assign, ":", 3)
		var name string
		num := "DEFAULT"
		mode := assignmentsv1alpha1.NodeAssignmentModeLabelOnly
		switch len(parts) {
		case 1:
			name = parts[0]
		case 2:
			name = parts[0]
			num = parts[1]
		case 3:
			name = parts[0]
			num = parts[1]
			mode = assignmentsv1alpha1.NodeAssignmentMode(parts[2])
		default:
			app.FatalUsage("Invalid assignment argument: %s\n", assign)
		}

		if num == "DEFAULT" {
			nag.Spec.DefaultAssignment = &assignmentsv1alpha1.NodeAssignment{
				Name: name,
				Mode: mode,
			}
			continue
		}

		if strings.HasSuffix(num, "%") {
			percentDesired, err := strconv.ParseInt(strings.TrimSuffix(num, `%`), 10, 32)
			if err != nil {
				app.FatalUsage("Error parsing assignment NUM: %s\n", err)
			}
			nag.Spec.Assignments = append(nag.Spec.Assignments, assignmentsv1alpha1.NodeAssignment{
				Name:           name,
				PercentDesired: int(percentDesired),
				Mode:           mode,
			})
			continue
		}

		if matched, _ := regexp.MatchString(`^\d+$`, num); matched {
			numDesired, err := strconv.ParseInt(strings.TrimSuffix(num, `%`), 10, 32)
			if err != nil {
				app.FatalUsage("Error parsing assignment NUM: %s\n", err)
			}
			nag.Spec.Assignments = append(nag.Spec.Assignments, assignmentsv1alpha1.NodeAssignment{
				Name:       name,
				NumDesired: int(numDesired),
				Mode:       mode,
			})
			continue
		}

		app.FatalUsage("Unsupported value for assignment NUM: %s\n", num)
	}

	if *dryRun {
		nag.Kind = assignmentsv1alpha1.NodeAssignmentGroupResourceKind
		nag.APIVersion = assignmentsv1alpha1.SchemeGroupVersion.Group + "/" + assignmentsv1alpha1.SchemeGroupVersion.Version
		outputObject(nag)
	} else {
		_, err := valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Create(nag)
		if err == nil {
			fmt.Printf("NodeAssignmentGroup %s created\n", *groupCreateCmdName)
		} else {
			app.Fatalf("Unable to create new NodeAssignmentGroup: %s", err)
		}
	}
}

func groupReportByNag() {
	var nags []assignmentsv1alpha1.NodeAssignmentGroup
	fetchErrors := make(map[string]error)

	if len(*groupReportNagsCmdNames) == 0 {
		nagResult, err := valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().List(metav1.ListOptions{})
		if err != nil {
			app.Fatalf("Error fetching NodeAssignmentGroups: %s", err)
		}
		nags = nagResult.Items
	} else {
		for _, nagName := range *groupReportNagsCmdNames {
			nagResult, err := valetClient.AssignmentsV1alpha1().NodeAssignmentGroups().Get(nagName, metav1.GetOptions{})
			if err != nil {
				fetchErrors[nagName] = err
			} else {
				nags = append(nags, *nagResult)
			}
		}
	}

	// Don't fetch nodes if there are no nags to report on
	if len(nags) != 0 {
		nodeResult, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			app.Fatalf("Error fetching Nodes: %s", err)
		}

		for _, nag := range nags {
			// Generate data
			// Example label : nag.assignments.kube-valet.io/NAGNAME=ASSIGNMENTNAME
			nagLabelKey := fmt.Sprintf("nag.assignments.kube-valet.io/%s", nag.GetName())
			assignmentMembers := make(map[string][]string)
			for _, node := range nodeResult.Items {
				for lk, lv := range node.GetLabels() {
					if lk == nagLabelKey {
						assignmentMembers[lv] = append(assignmentMembers[lv], node.GetName())
					}
				}
			}

			// Report results
			assignments := []string{}
			numRows := 0
			for k, v := range assignmentMembers {
				assignments = append(assignments, k)
				if len(v) > numRows {
					numRows = len(v)
				}
			}

			fmt.Printf("========= NAG %s Assignments =========\n", nag.GetName())
			// setup table writer
			const padding = 3
			w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, ' ', 0)
			fmt.Fprintln(w, "--"+strings.Join(assignments, "--\t--")+"--\t")
			for i := 0; i < numRows; i++ {
				for _, assign := range assignments {
					if len(assignmentMembers[assign]) > 0 {
						var member string
						member, assignmentMembers[assign] = assignmentMembers[assign][0], assignmentMembers[assign][1:]
						fmt.Fprintf(w, "%s\t", member)
					} else {
						fmt.Fprintf(w, "\t")
					}
				}
				fmt.Fprintf(w, "\t\n")
			}

			w.Flush()

			fmt.Println()

			// Report errors
			for k, v := range fetchErrors {
				fmt.Fprintf(os.Stderr, "Error fetching %s: %s\n", k, v)
			}
		}
	} else {
		fmt.Println("No resources found.")
	}
}

func groupReportByNode() {
	var nodes []corev1.Node
	fetchErrors := make(map[string]error)

	if len(*groupReportNodesCmdNames) == 0 {
		nodeResult, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			app.Fatalf("Error fetching Nodes: %s", err)
		}
		nodes = nodeResult.Items
	} else {
		for _, nodeName := range *groupReportNodesCmdNames {
			nodeResult, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if err != nil {
				fetchErrors[nodeName] = err
			} else {
				nodes = append(nodes, *nodeResult)
			}
		}
	}

	// setup table writer
	const padding = 3
	w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, ' ', 0)
	fmt.Fprintln(w, "NODE\tNAG-ASSIGNMENTS\tNAG-TAINTS\t")

	for _, node := range nodes {
		var assignLabels, assignTaints []string

		// Check for protected nodes
		if protectedLabelVal, ok := node.GetLabels()[assignmentsv1alpha1.ProtectedNodeLabelKey]; ok && protectedLabelVal == assignmentsv1alpha1.ProtectedLabelValue {
			// set the assignLabels and assignTaints to PROTECTED but don't return yet. If something is wrong with kube-valet, it makes sense to always show any other
			// assignments that may have been set rather than hide it by exiting early.
			assignLabels = []string{"PROTECTED"}
			assignTaints = []string{"PROTECTED"}
		}

		// Example label : nag.assignments.kube-valet.io/NAGNAME=ASSIGNMENTNAME
		for lk, lv := range node.GetLabels() {
			if strings.HasPrefix(lk, "nag.assignments.kube-valet.io/") {
				parts := strings.SplitN(lk, "/", 2)
				assignLabels = append(assignLabels, fmt.Sprintf("%s/%s", parts[1], lv))
			}
		}
		for _, taint := range node.Spec.Taints {
			if strings.HasPrefix(taint.Key, "nag.assignments.kube-valet.io/") {
				parts := strings.SplitN(taint.Key, "/", 2)
				assignTaints = append(assignTaints, fmt.Sprintf("%s/%s:%s", parts[1], taint.Value, taint.Effect))
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t\n", node.GetName(), strings.Join(assignLabels, ","), strings.Join(assignTaints, ","))
	}

	// Report results
	w.Flush()
}

func assignmentCreate() {
	targetLabels, err := labels.ConvertSelectorToLabelsMap(*assignmentCreateCmdTargetLabels)
	if err != nil {
		app.FatalUsage("Error parsing targetLabels: %s\n", err)
	}

	spec := assignmentsv1alpha1.PodAssignmentRuleSpec{
		TargetLabels: targetLabels,
		Scheduling: assignmentsv1alpha1.PodAssignmentRuleScheduling{
			MergeStrategy: assignmentsv1alpha1.PodAssignmentRuleSchedulingMergeStrategyDefault,
		},
	}

	// Parse nag
	if (*assignmentCreateCmdMode == "prefer" || *assignmentCreateCmdMode == "require") && *assignmentCreateCmdAssignment != "" {
		parts := strings.Split(*assignmentCreateCmdAssignment, "/")

		term := corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key: "nag.assignments.kube-valet.io/" + parts[0],
				},
			},
		}

		switch len(parts) {
		case 2:
			term.MatchExpressions[0].Operator = corev1.NodeSelectorOpIn
			term.MatchExpressions[0].Values = strings.Split(parts[1], ",")
			for _, v := range strings.Split(parts[1], ",") {
				spec.Scheduling.Tolerations = append(spec.Scheduling.Tolerations, corev1.Toleration{
					Key:      "nag.assignments.kube-valet.io/" + parts[0],
					Operator: corev1.TolerationOpEqual,
					Value:    v,
					// Match all effects by not giving an Effect key
				})
			}
		case 1:
			term.MatchExpressions[0].Operator = corev1.NodeSelectorOpExists
			spec.Scheduling.Tolerations = []corev1.Toleration{
				{
					Key:      "nag.assignments.kube-valet.io/" + parts[0],
					Operator: corev1.TolerationOpExists,
					// Match all effects by not giving an Effect key
				},
			}
		default:
			app.FatalUsage("Invalid assignment: %s", *assignmentCreateCmdAssignment)
		}

		applyTermToSpec(term, &spec)
	}

	// Parse nodeaffinity
	if (*assignmentCreateCmdMode == "prefer" || *assignmentCreateCmdMode == "require") && *assignmentCreateCmdNodeAffinity != "" {
		parts := strings.Split(*assignmentCreateCmdNodeAffinity, "=")

		term := corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key: parts[0],
				},
			},
		}

		switch len(parts) {
		case 2:
			term.MatchExpressions[0].Operator = corev1.NodeSelectorOpIn
			term.MatchExpressions[0].Values = strings.Split(parts[1], ",")
		case 1:
			term.MatchExpressions[0].Operator = corev1.NodeSelectorOpExists
		default:
			app.FatalUsage("Invalid NodeAffinity target: %s", *assignmentCreateCmdAssignment)
		}

		applyTermToSpec(term, &spec)
	}

	// Parse node assignment labels
	if *assignmentCreateCmdMode == "nodes" && *assignmentCreateCmdNodeSelector != "" {
		nodeSelector, err := labels.ConvertSelectorToLabelsMap(*assignmentCreateCmdNodeSelector)
		if err != nil {
			app.FatalUsage("Error parsing nodeSelector labels: %s\n", err)
		}
		spec.Scheduling.NodeSelector = nodeSelector
	}

	// Parse PodAffinity prefer
	if *assignmentCreateCmdMode == "prefer-others" || *assignmentCreateCmdMode == "require-others" || *assignmentCreateCmdMode == "avoid-others" || *assignmentCreateCmdMode == "deny-others" {
		matchExpressions := []metav1.LabelSelectorRequirement{}

		for k, v := range targetLabels {
			matchExpressions = append(matchExpressions, metav1.LabelSelectorRequirement{
				Key:      k,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{v},
			})
		}

		paTerm := corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{
				MatchExpressions: matchExpressions,
			},
			TopologyKey: *assignmentCreateCmdTopologyKey,
		}

		// Init afffinity pointer
		spec.Scheduling.Affinity = &corev1.Affinity{}

		if *assignmentCreateCmdMode == "prefer-others" {
			spec.Scheduling.Affinity.PodAffinity = &corev1.PodAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight:          100,
						PodAffinityTerm: paTerm,
					},
				},
			}
		}

		if *assignmentCreateCmdMode == "require-others" {
			spec.Scheduling.Affinity.PodAffinity = &corev1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{paTerm},
			}
		}

		if *assignmentCreateCmdMode == "avoid-others" {
			spec.Scheduling.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight:          100,
						PodAffinityTerm: paTerm,
					},
				},
			}
		}

		if *assignmentCreateCmdMode == "deny-others" {
			spec.Scheduling.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{paTerm},
			}
		}
	}

	// If the namespace flag was given, create a PodAssignmentRUle
	if *assignmentCreateCmdNamespace != "" {
		par := &assignmentsv1alpha1.PodAssignmentRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *assignmentCreateCmdName,
				Namespace: *assignmentCreateCmdNamespace,
			},
			Spec: spec,
		}

		if *dryRun {
			par.Kind = assignmentsv1alpha1.PodAssignmentRuleResourceKind
			par.APIVersion = assignmentsv1alpha1.SchemeGroupVersion.Group + "/" + assignmentsv1alpha1.SchemeGroupVersion.Version
			outputObject(par)
		} else {
			_, err := valetClient.AssignmentsV1alpha1().PodAssignmentRules(*assignmentCreateCmdNamespace).Create(par)
			if err == nil {
				fmt.Printf("PodAssignmentRule %s created in %s\n", *groupCreateCmdName, *assignmentCreateCmdNamespace)
			} else {
				app.Fatalf("Unable to create new ClusterPodAssignmentRule: %s", err)
			}
		}
	} else { // If the namespace flag not given, create a ClusterPodAssignmentRUle
		cpar := &assignmentsv1alpha1.ClusterPodAssignmentRule{
			ObjectMeta: metav1.ObjectMeta{
				Name: *assignmentCreateCmdName,
			},
			Spec: spec,
		}

		if *dryRun {
			cpar.Kind = assignmentsv1alpha1.ClusterPodAssignmentRuleResourceKind
			cpar.APIVersion = assignmentsv1alpha1.SchemeGroupVersion.Group + "/" + assignmentsv1alpha1.SchemeGroupVersion.Version
			outputObject(cpar)
		} else {
			_, err := valetClient.AssignmentsV1alpha1().ClusterPodAssignmentRules().Create(cpar)
			if err == nil {
				fmt.Printf("ClusterPodAssignmentRule %s created\n", *groupCreateCmdName)
			} else {
				app.Fatalf("Unable to create new ClusterPodAssignmentRule: %s", err)
			}
		}
	}
}

func outputObject(o interface{}) {
	switch *output {
	case "json":
		data, err := json.MarshalIndent(o, "", "    ")
		if err != nil {
			app.Fatalf("Unable to marshal object: %s", err)
		}
		fmt.Printf("%s\n", data)
	case "yaml":
		data, err := yaml.Marshal(o)
		if err != nil {
			app.Fatalf("Unable to marshal object: %s", err)
		}
		fmt.Printf("%s", data)
	}
}

func applyTermToSpec(term corev1.NodeSelectorTerm, spec *assignmentsv1alpha1.PodAssignmentRuleSpec) {
	switch *assignmentCreateCmdMode {
	case "require":
		spec.Scheduling.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{term},
				},
			},
		}
	case "prefer":
		spec.Scheduling.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
					{
						Weight:     100,
						Preference: term,
					},
				},
			},
		}
	default:
		app.Fatalf("Unable to apply term for mode: %s", *assignmentCreateCmdMode)
	}
}
