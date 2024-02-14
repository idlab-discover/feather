#!/bin/sh

# Set kubeconfig
export KUBECONFIG=$HOME/.kube/fledge2.yml

# Deploy container
kubectl delete pods/example-image
kubectl apply -f - << EOF
apiVersion: v1
kind: Pod
metadata:
  name: example-image
spec:
  containers:
  - name: example-image
    image: gitlab.ilabt.imec.be:4567/fledge/flint/example-image:latest
  nodeSelector:
    type: virtual-kubelet
  tolerations:
  - key: virtual-kubelet.io/provider
    operator: Equal
    value: backend
    effect: NoSchedule
EOF
