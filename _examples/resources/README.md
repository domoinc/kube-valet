# Example Resources

This directory contains example resources used to configure kube-valet.

## Resources

### NodeAssignmentGroups

These resources are used to automatically divide up node capacity. See the [NodeAssignmentGroups](./nodeassignmentgroups/) folder for more information.

### PodAssignmentRules and ClusterPodAssignmentRules

These resources are used to automatically update pod scheduling data as the pods are created in the cluster. The resources are identical, the only difference being the scope to which they apply. See the [PodAssignmentRules](./podassignmentrules/) and [ClusterPodAssignmentRules](./clusterpodassignmentrules) folders for more information.
