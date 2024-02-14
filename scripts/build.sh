#!/bin/sh

# Enter project root
cd "$(dirname "$(go env GOMOD)")" || { echo "error: can't enter module directory"; exit 1; }
set -e

# Get version flags
build_version="-X 'main.buildVersion=$(git describe --tags --always --dirty="-dev")'"
build_time="-X 'main.buildTime=$(date -u '+%Y-%m-%d-%H:%M UTC')'"
linker_flags="$build_version $build_time"
build_flags=""

# Build for multiple architectures
export CGO_ENABLED=0
export GOOS=linux
for GOARCH in amd64; do #arm64 amd64; do
  # Build statically linked binary
  dir="./out/${GOOS}/${GOARCH}"
  # Compile debug version
  go build -a -gcflags="${build_flags}" -o "${dir}/fledge.debug" -ldflags="${linker_flags}" "./cmd/fledge"
  # Strip binary and compile statically
  go build -a -gcflags="${build_flags}" -o "${dir}/fledge" -ldflags="-s -w ${linker_flags}" "./cmd/fledge"
done
