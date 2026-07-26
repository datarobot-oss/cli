#!/usr/bin/env bash
set -euo pipefail

# Install Task if the devcontainer feature didn't
if ! command -v task &> /dev/null; then
    echo "Installing Task runner..."
    sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
fi

# Upgrade Go if the image version doesn't match go.mod
GO_REQUIRED=$(grep '^go ' go.mod | awk '{print $2}')
GO_INSTALLED=$(go version | awk '{print $3}' | sed 's/go//')
if [ "$GO_REQUIRED" != "$GO_INSTALLED" ]; then
    echo "Go mismatch: need $GO_REQUIRED, have $GO_INSTALLED. Upgrading..."
    ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
    rm -rf /usr/local/go
    curl -sL "https://go.dev/dl/go${GO_REQUIRED}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
fi

task bootstrap
