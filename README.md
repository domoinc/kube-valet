# Kube-Valet: Your Cluster Parking Assistant

Kube-valet is a [custom controller](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#custom-controllers) for Kubernetes. It allows cluster administrators or users to:

  * [Dynamically allocate cluster capacity for specific workloads](./_examples/resources/nodeassignmentgroups/)
  * [Automatically apply scheduling configuration to pods after they are created](./_examples/resources/podassignmentrules/)

It does this by utilizing
[CustomResourceDefinitions](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#customresourcedefinitions) to define its behavior and a [MutatingWebhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
to modify pods as they are created.

Kube-valet is currently in an **alpha** state.

## Requirements

* Kubernetes v1.13 or greater
* The MutatingWebhook AdmissionController must be enabled (Default on most clusters)
* The admissionregistration.k8s.io api group must be enabled (Default on most clusters)
* Cluster administrator level access

## Install Using Helm and Automatic TLS

This is the easiest and fastest way to get started with kube-valet.

```bash
git clone git@github.com:domoinc/kube-valet.git
cd kube-valet
helm install -n kube-valet --set tls.auto=true --wait ./helm
```

It is normal for the automatic tls installation to take a few minutes
while the images download and all the resource become ready.

## Install Using Helm or YAML and Manual TLS

**Note:** The instructions below are for generating a new self-signed certficates.
This is done using CFSSL: Cloudflare's PKI and TLS toolkit. [Click here](https://blog.cloudflare.com/introducing-cfssl/) to know more.
The cfssl tools used can be downloaded at https://pkg.cfssl.org/.

### Get The Repository

```bash
# Clone the repository and cd into it
git clone git@github.com:domoinc/kube-valet.git
cd kube-valet
```

### Generate Self-Signed TLS Certificates

```bash
# Make a dedicated dir for the certificates and cd into it
mkdir tls
cd tls

# Init a ca with cfssl
cat <<EOF | cfssl genkey -initca - | cfssljson -bare ca
{
    "CN": "kube-valet-ca",
    "key": {
        "algo": "ecdsa",
        "size": 256
    }
}
EOF

# Create a self-signed key and certificate
cat <<EOF | cfssl gencert -ca ca.pem -ca-key ca-key.pem - | cfssljson -bare server
{
    "CN": "kube-valet.kube-valet.svc",
    "hosts": [
        "kube-valet.kube-valet.svc",
        "kube-valet.kube-valet.svc.cluster.local"
    ],
    "key": {
        "algo": "ecdsa",
        "size": 256
    }
}
EOF

# Go back to the root of the project
cd ../
```

### Install with Helm and Manual TLS

```bash
# Put the certificates in the location expected by the helm templates by default
cp -r tls helm/

# Do helm install using default cert, ca, and key paths
helm install -n kube-valet --wait ./helm
```

### Install with YAML and Manual TLS

```bash
# Replace the cert and key placeholders in the valet secret
vim deploy/secret.yaml

# Replace the ca cert placeholder in the webhook config
vim deploy/mutatingwebhookconfiguration.yaml

# Apply the namespace
kubectl apply -f deploy/namespace.yaml

# Apply the deployment files
kubectl apply -f deploy/
```

---

## Using Kube-Valet

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

Valetctl is a tool that makes it easier to create and report on kube-valet resources.

### Install

```bash
curl -Lo /usr/local/bin/valetctl https://github.com/domoinc/kube-valet/releases/download/v2018.10.17.0/valetctl
chmod +x /usr/local/bin/valetctl
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

# Create a Namespace
kubectl create namespace kv-example

# Enable kube-valet in the namespace (not required for global installations)
kubectl label namespace kv-example kube-valet.io/enabled=""

# Report by nodes
valetctl group report nodes

# Run some jobtype=sensitive pods
kubectl -n kv-example run sensitive --image=alpine:latest sleep 36000 -l jobtype=sensitive --replicas=5

# Check which nodes are in the sensitive assignment column
valetctl group report nags for-jobs

# Check that the all of the pods went to the expected nodes automatically
kubectl -n kv-example get pods -l jobtype=sensitive -o wide

# Run some jobtype=relaxed pods
kubectl -n kv-example run relaxed --image=alpine:latest sleep 36000 -l jobtype=relaxed --replicas=5

# Check which nodes are in the relaxed assignment column
valetctl group report nags for-jobs

# Check that all or most of the pods went to the expected nodes automatically
kubectl -n kv-example get pods -l jobtype=relaxed -o wide

# Run some jobtype=misc pods
kubectl -n kv-example run misc --image=alpine:latest sleep 36000 -l jobtype=misc --replicas=5

# Check which nodes are in any assignment
valetctl group report nags for-jobs

# Check that all or most of the pods went to the expected nodes automatically
kubectl -n kv-example get pods -l jobtype=misc -o wide
```

Clean up the test resources

```bash
kubectl delete nag for-jobs
kubectl delete cpar sensitivereq relaxedpref miscpref
kubectl delete namespace kv-example
```

## Protecting Resources

It is possible to instruct kube-valet to always ignore specific pods or nodes.

### Protecting Pods

  * Kube-valet will ignore pods with a label of `pod.initializer.kube-valet.io/protected=true`

### Protecting Nodes

  * Kube-valet will ignore nodes with a label of `nags.kube-valet.io/protected=true`

## Local Development

### Requirements

  * `make`
  * Golang 1.12+

### Process

  * Code change
  * `make` to build everything or `make build` to just build kube-valet and valetctl.
  * Run via `./build/kube-valet --kubeconfig ~/.kube/config --loglevel="DEBUG" --no-leader-elect --key=/path/to/key.pem --cert=/path/to/cert.pem`

### Building a Release Image

  * `make docker-image`
  * `make push-docker-image`

## Contributing

PRs and issues are encouraged and welcome.

## Community Code of Conduct

Kube-valet follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

## Credits

Copyright (c) 2018 Domo, Inc.

Kube-valet was originally created by Carson Anderson ([@carsonoid](https://github.com/carsonoid)) with additional help from Jordan Davidson ([@from-nibly](https://github.com/from-nibly))
