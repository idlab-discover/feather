#!/bin/sh

# Set kubeconfig
export KUBECONFIG=$HOME/.kube/fledge2.yml
export IMAGE=minimal
export VERSION=1.0.0
export BACKEND=capstan

# Deploy container
sudo kubectl delete "deployments/$IMAGE-$BACKEND"
sudo kubectl apply -f - << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $IMAGE-$BACKEND
spec:
  selector:
    matchLabels:
      run: $IMAGE-$BACKEND
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
  template:
    metadata:
      labels:
        run: $IMAGE-$BACKEND
    spec:
      containers:
      - name: app
        image: gitlab.ilabt.imec.be:4567/fledge/benchmark/$IMAGE:$VERSION-$BACKEND
        imagePullPolicy: Always
        resources:
          limits:
            cpu: 1
            memory: 1Gi
      nodeSelector:
        type: virtual-kubelet
      tolerations:
      - key: virtual-kubelet.io/provider
        operator: Equal
        value: backend
        effect: NoSchedule
EOF
