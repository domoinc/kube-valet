# Kube-Valet: Your Cluster Parking Assistant

Kube-valet is a [custom controller](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#custom-controllers) for Kubernetes. It allows cluster administrators or users to:

  * [Dynamically allocate cluster capacity for specific workloads](./_examples/resources/nodeassignmentgroups/)
  * [Automatically apply scheduling configuration to pods after they are created](./_examples/resources/podassignmentrules/)

It does this by utilizing
[CustomResourceDefinitions](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#customresourcedefinitions) to define its behavior and an [Initializer](https://kubernetes.io/docs/admin/extensible-admission-controllers/#initializers)
to modify pods after they are created but before they are scheduled.

Kube-valet is currently in an **alpha** state.

## Setup

```bash
# Clone the repository and cd into it
git clone git@github.com:domoinc/kube-valet.git
cd kube-valet

# Create the CustomResourceDefinitions used by kube-valet
kubectl create -f deploy/customresourcedefinitions

# Create the ServiceAccount
kubectl create -f deploy/serviceaccount.yaml

# If the cluster has RBAC enabled, create the RBAC resources
kubectl create -f deploy/rbac.yaml

# Protect the masters from being included in any NodeAssignmentGroups (skip this if running minikube)
kubectl label node -l node-role.kubernetes.io/master nags.kube-valet.io/protected=true
```

## Run the Controller

### As a Static Pod on all Masters

Because kube-valet uses initializers to modify all pods before they run, the prefered method of execution is as a [Static Pod](https://kubernetes.io/docs/tasks/administer-cluster/static-pod/). Static pods are not subject to the intializer admission controller. Kube-valet also has built-in leader election so that there is only one active valet even if multiple pods are running at the same time.

Copy the static pod manifest to the nodes or masters that will be running a copy of kube-valet.

```bash
# From git
curl -Lo /etc/kubernetes/manifests/kube-valet.yaml https://github.com/domoinc/kube-valet/deploy/static-pod.yaml

# From the locally cloned repository
cp deploy/static-pod.yaml /etc/kubernetes/manifests/kube-valet.yaml
```

### As a Deployment

**WARNING** While it's easier to kube-valet as a deployment, there are possible situations  where no pods will be initialized because kube-valet is not running to initilize itself. The deployment is constructed to minimize this possiblity, but it can still happen.

```bash
kubectl create -f deploy/deployment.yaml
```

#### Resolving a Self-Initialization Stalemate

If the initializerconfiguration has already been created, or if all of the kube-valet pods ever get deleted at the same time, The kube-valet member pods must be manually initialized by editing them and removing `.metadata.initializers` list or with the following command:

```bash
kubectl --namespace=kube-system get po --include-uninitialized -l k8s-app=kube-valet -o name | xargs -n1 kubectl --namespace=kube-system patch --type=json -p='[{"op":"remove","path":"/metadata/initializers/pending/0"}]'
```

If possible, the command above should be scheduled to run on a regular basis just to make sure that a self-initialization stalement is automatically resolved.

### Enable the Initializer

**WARNING** Do not do this until kube-valet is running or the manual initializion command is running as a scheduled task.

First, [Enable Intializers](https://kubernetes.io/docs/admin/extensible-admission-controllers/#enable-initializers-alpha-feature) in the `kube-apiserver`

Then Create the initalizerconfiguration for pods:

```bash
kubectl create -f deploy/initializerconfiguration.yaml
```

## Use Kube-Valet

Kube-valet is configured entirely through the custom resources made during installation. The resources can be created, updated, and deleted just like any other kubernetes resource. Example:

```bash
kubectl create -f _examples/resources/nodeassignmentgroups/simple.yaml
kubectl get nags
kubectl delete nag simple
```

Available custom resources:

  * pars  - podassignmentrules
  * cpars - clusterpodassignmentrules
  * nags  - nodeassignmentgroups

See the [examples](./_examples) for example client-go scripts and detailed custom resource examples.

## Use Valetctl to Configure Kube-Valet

valetctl is a tool that makes it easier to create and report on kube-valet resources.

### Install

```bash
curl -Lo /usr/local/bin/valetctl http://RELEASEURL
```

### Usage Example

Set aside some nodes and make specific pods target those nodes.

```bash
# Isolate one node for sensitive jobs and label two more for relaxed jobs
valetctl group create for-jobs sensitive:1:LabelAndTaint relaxed:2

# Make sure all jobtype=sensitive pods in any namespace run on the isolated node.
valetctl assignment create sensitivereq require -t jobtype=sensitive -A for-jobs/sensitive

# Make sure all jobtype=relaxed pods prefer to run on the separated nodes
valetctl assignment create relaxedpref prefer -t jobtype=relaxed -A for-jobs/relaxed

# Make sure all jobtype=misc pods prefer to run any for-jobs assignments
valetctl assignment create miscpref prefer -t jobtype=misc -A for-jobs
```

Make pods that match the rules created above and prove that they are on the designated nodes.

```bash
# Report by nodes
valetctl group report nodes

# Run some jobtype=sensitive pods
kubectl run sensitive --image=alpine:latest sleep 36000 -l jobtype=sensitive --replicas=5

# Check which nodes are in the sensitive assignment column
valetctl group report nags for-jobs

# Check that the all of the pods went to the expected nodes automatically
kubectl get pods -l jobtype=sensitive -o wide

# Run some jobtype=relaxed pods
kubectl run relaxed --image=alpine:latest sleep 36000 -l jobtype=relaxed --replicas=5

# Check which nodes are in the relaxed assignment column
valetctl group report nags for-jobs

# Check that all or most of the pods went to the expected nodes automatically
kubectl get pods -l jobtype=relaxed -o wide

# Run some jobtype=misc pods
kubectl run misc --image=alpine:latest sleep 36000 -l jobtype=misc --replicas=5

# Check which nodes are in any assignment
valetctl group report nags for-jobs

# Check that all or most of the pods went to the expected nodes automatically
kubectl get pods -l jobtype=misc -o wide
```

Clean up the test resources

```bash
kubectl delete nag for-jobs
kubectl delete cpar sensitivereq relaxedpref miscpref
kubectl delete deployment sensitive relaxed misc
```

## Protecting Resources

It is possible to instruct kube-valet to always ignore specific pods or nodes.

### Protecting Pods

  * Kube-valet will ignore pods with a label of `pod.initializer.kube-valet.io/protected=true`

### Protecting Nodes

  * Kube-valet will ignore nodes with a label of `nags.kube-valet.io/protected=true`

## Disaster Recovery

If pods are not showing up via normal `kubectl` commands, check for unintialized pods by adding the `--include-uninitialized` flag. Example:

```bash
kubectl get pods --include-uninitialized --all-namespaces
```

If kube-valet is not running or not working and pods are getting stuck in an un-initalized state: Follow the [Disaster Recovery docs](./docs/DisasterRecovery.md) to disable the initializer and manually initialize all pods.

## Local Development

### Requirements

  * `make`
  * Golang 1.9+
  * [glide](https://github.com/Masterminds/glide)

### Process

  * Code change
  * `make`
  * Run via `./build/kube-valet --kubeconfig ~/.kube/config --loglevel="DEBUG" --no-leader-elect`

### Building a Release Image

  * `make docker-image`
  * `make push-docker-image`

## Contributing

PRs and issues are encouraged and welcome.

## Community Code of Conduct

Kube-valet follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

## Credits

Copyright (c) 2018 Domo, Inc.

Kube-valet was originally created by Carson Anderson ([@carsonoid](https://github.com/carsonoid)) with additional help from Jordan Davidson [@from-nibly](https://github.com/from-nibly)
