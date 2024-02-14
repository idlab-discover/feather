#!/bin/sh

# Enter project root
cd "$(dirname "$(go env GOMOD)}")" || { echo "error: can't enter module directory"; exit 1; }
set -e

# Build binary
"./scripts/build.sh"

# Copy binary
# ssh worker0 pkill fledge || true
scp "default.json" "backend.json" "broker.json" "./out/$(go env GOHOSTOS)/$(go env GOHOSTARCH)/fledge" "worker0:~"

# Run FLEDGE
ssh -t worker0 sudo KUBECONFIG=~/.kube/kubeconfig.yml KUBERNETES_SERVICE_HOST=10.2.0.118 KUBERNETES_SERVICE_PORT=6443 ./fledge $@ | grep --color=always -v 'http: TLS handshake error from'
