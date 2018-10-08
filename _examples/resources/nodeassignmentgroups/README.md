# NodeAssignmentGroups

This custom resource can be used to dynamically label and/or taint nodes. This often pairs well with the (Cluster)PodAssignmentRules as a way to distribute load among nodes transparently to users.

You can request a static number of nodes for a specific purpose, a percentage of nodes, [all nodes](./nodeassignmentgroups/default.yaml), or even do more advanced scheduling like [PackLeft](./nodeassignmentgroups/packleft.yaml).


## v1Alpha1 Format

The document below describes all possible fields. For more specific examples, please check the other example yaml files in this directory.

```yaml
apiVersion: assignments.kube-valet.io/v1alpha1
kind: NodeAssignmentGroup
metadata:
  name: preference
spec:
  # targetLabels is optional. It is used to define the "group" of nodes that will be monitored and modified
  # if it is not given than the rule will apply to -all- nodes in the cluster. Including the master
  # nodes if they have registered with the api.
  targetLabels:
    node-role.kubernetes.io/worker: "" # Explicitly target non-master nodes. This label is assumed to have been set by the cluster admin on node creation
  # assignments is optional. It is a prioritized list so if there are not enough nodes for all assignments than
  # it will take from lower assignments to allocate for higher assignments
  # Labels and/or taints for assignments use the NodeAssignmentGroup name and assignment name to generate the key/value pairs:
  # the format is:
  #   label:  nag.assignments.kube-valet.io/<NodeAssignmentGroup Name>="<Assignment Name>"
  #   taint:  nag.assignments.kube-valet.io/<NodeAssignmentGroup Name>="<Assignment Name>:<Assignment Taint taintEffect>"
  assignments:
    - name: jobs
      mode: LabelAndTaint # Optional. Valid choices: LabelOnly, LabelAndTaint. Default: LabelOnly
      taintEffect: PreferNoSchedule # Optional. Valid choices are any upstream TaintEffects for the Pod Spec. Default: NoSchedule
      numDesired: 1 # Optional. Default: 0
  # defaultAssignment is optional.
  # When given any nodes that are left in the group after processing assignments will get the assignment provided:
  #   label:  nag.assignments.kube-valet.io/preference="none"
  defaultAssignment:
    name: none # All assignment fields except numDesired are supported.
    mode: LabelOnly
    taintEffect: NoSchedule
```

