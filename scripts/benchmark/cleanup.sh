#!/bin/sh

# Set kubeconfig
export KUBECONFIG=$HOME/.kube/fledge2.yml
export IMAGE=minimal
export VERSION=1.0.0
export BACKEND=capstan

# Deploy container
kubectl delete "deployments/minimal-docker"
kubectl delete "deployments/minimal-capstan"
kubectl delete "configmaps/papermc"
kubectl delete "deployments/papermc-docker"
kubectl delete "deployments/papermc-capstan"
