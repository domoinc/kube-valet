# ClusterPodAssignmentRules

This custom resource can be used to dynamically apply scheduling information to pods as they are created. These resources are created at the namespace scope. For namespace-scoped matching, use [PodAssignmentRules](../podassignmentrules).

## v1Alpha1 Format

The document below describes all possible fields. For more specific examples, please check the other example yaml files in this directory. Kube-valet simply applies the upstream Kubernetes [node selection](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/) and/or [tolerations](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/) data.

```yaml
apiVersion: assignments.kube-valet.io/v1alpha1
kind: ClusterPodAssignmentRule
metadata:
  name: preference
spec:
  # targetLabels is optional. It is used to define which the rule should apply to
  # if it is not given than the rule will apply to -all- pods in any namespace.
  targetLabels:
    worktype: complex
  # The scheduling key holds all possible scheduling data for the pods.
  scheduling:
    # MergeStrategy tells kube-valet how to apply the rules when they match. The default is to overwrite
    # anything in the pod with the rules provided.
    mergeStrategy: OverwriteAll
    # The nodeSelector key contains the upstream Kubernetes nodeSelector type
    nodeSelector:
      worktype: complex
    # The affinity key contains the upstream Kubernetes affinity type
    affinity:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: kubernetes.io/e2e-az-name
              operator: In
              values:
              - e2e-az1
              - e2e-az2
        preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 1
          preference:
            matchExpressions:
            - key: another-node-label-key
              operator: In
              values:
              - another-node-label-value
    # The tolerations key contains the upstream Kubernetes tolerations type
    tolerations:
    - key: "key"
      operator: "Equal"
      value: "value"
      effect: "NoSchedule"
```

## Protecting Pods from Modification

To make sure that a pod is never modified by kube-valet, regardless of any matching rules. The pod can be given the `pod.initializer.kube-valet.io/protected=true` label. Which instructs kube-valet to simply initialize the pod without modification. This is useful for system pods that should be safe from modification.
