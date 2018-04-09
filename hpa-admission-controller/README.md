# VPA Admission Controller

## Intro

This is a binary that registers itself as a Mutating Admission Webhook
and because of that is on the path of creating all Horizontal Pod Autoscalers.
For each of them, it'll format metric name for use with Custom/External Metrics
Stackdriver Adapter.

## Running

1. Make sure your API server supports Mutating Webhooks.
The `--admission-control` flag should have `MutatingAdmissionWebhook` as one of
the values on the list and the `--runtime-config` flag should include
`admissionregistration.k8s.io/v1beta1=true`.
To change those flags, SSH to your master instance, edit
`/etc/kubernetes/manifests/kube-apiserver.manifest` and restart kubelet to pick
up the changes: ```sudo systemctl restart kubelet.service```
1. Generate certs by running `bash gencerts.sh`. This will use kubectl to create
   a secret in your cluster with the certs.
1. Create RBAC configuration for the admission controller pod by running
   `kubectl create -f ../deploy/admission-controller-rbac.yaml`
1. Create the pod:
   `kubectl create -f ../deploy/admission-controller-deployment.yaml`.
   The first thing this will do is register itself as a Mutating Admission Webhook.
   Then, it will format metric name in horizontal pod autoscalers on their creation & updates.


