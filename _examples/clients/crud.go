package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
)

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := valet.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	example := &assignmentsv1alpha1.ClusterPodAssignmentRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testassign",
		},
		Spec: assignmentsv1alpha1.PodAssignmentRuleSpec{
			TargetLabels: labels.Set{
				"test": "true",
			},
			Scheduling: assignmentsv1alpha1.PodAssignmentRuleScheduling{
				MergeStrategy: assignmentsv1alpha1.PodAssignmentRuleSchedulingMergeStrategyDefault,
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "testkey",
											Operator: corev1.NodeSelectorOpIn,
											Values: []string{
												"testval",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(example)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", data)
	fmt.Printf("%+v\n", example)

	result, err := clientset.AssignmentsV1alpha1().ClusterPodAssignmentRules().Create(example)
	if err == nil {
		fmt.Printf("\nCREATED: %#v\n\n", result)
	} else {
		panic(err)
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of the fleet before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := clientset.AssignmentsV1alpha1().ClusterPodAssignmentRules().Get("testassign", metav1.GetOptions{})
		fmt.Printf("result: %+v\n", result)
		if getErr != nil {
			fmt.Printf("Failed to get latest version of clusterpodassignment: %v\n", getErr)
		}

		result.Spec.TargetLabels["test"] = "false"

		_, updateErr := clientset.AssignmentsV1alpha1().ClusterPodAssignmentRules().Update(result)
		return updateErr
	})
	if retryErr != nil {
		fmt.Printf("\nUpdate failed: %+v\n\n", retryErr)
	}
	fmt.Printf("\nUpdated clusterpodassignment...\n\n")

	// clientset.AssignmentsV1alpha1().ClusterPodAssignmentRules().Delete("testassign", &metav1.DeleteOptions{})
	// fmt.Printf("Deleted clusterpodassignment...\n")
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
