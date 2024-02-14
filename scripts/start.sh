#!/bin/sh

# Enter project root
cd "$(dirname "$(go env GOMOD)}")" || { echo "error: can't enter module directory"; exit 1; }
set -e

# Build binary
"./scripts/build.sh"

# Start binary
"./out/$(go env GOHOSTOS)/$(go env GOHOSTARCH)/fledge" $@
