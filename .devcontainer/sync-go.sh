#!/usr/bin/env bash
set -euo pipefail

# Upgrade Go if the image version doesn't match go.mod.
#
# The official SHA256 is fetched from Go's release JSON feed so no hash is
# hardcoded. The existing toolchain is only removed after the downloaded
# tarball has been verified, so a failed or partial download never leaves
# the container without Go (and updateContentCommand/postCreateCommand can
# still run).
GO_REQUIRED=$(grep '^go ' go.mod | awk '{print $2}')
GO_INSTALLED=$(go version | awk '{print $3}' | sed 's/go//')

# A newer toolchain satisfies an older go.mod directive, and the release
# feed only lists current versions — a superseded patch release cannot be
# fetched at all. So only ever upgrade forward.
if [ "$(printf '%s\n%s\n' "$GO_REQUIRED" "$GO_INSTALLED" | sort -V | head -n1)" = "$GO_REQUIRED" ]; then
    echo "Go $GO_INSTALLED satisfies go.mod's go $GO_REQUIRED"
elif [ "$GO_REQUIRED" != "$GO_INSTALLED" ]; then
    echo "Go mismatch: need $GO_REQUIRED, have $GO_INSTALLED. Upgrading..."
    ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
    TARBALL="go${GO_REQUIRED}.linux-${ARCH}.tar.gz"

    # Look up the official SHA256 from Go's release feed (nothing hardcoded).
    RELEASES=$(curl -fsSL "https://go.dev/dl/?mode=json")
    if command -v jq >/dev/null 2>&1; then
        EXPECTED_SHA=$(printf '%s' "$RELEASES" \
            | jq -r --arg f "$TARBALL" '.[] | .files[] | select(.filename==$f) | .sha256' \
            || true)
    else
        # grep fallback: the sha256 follows the matching filename in its object.
        EXPECTED_SHA=$(printf '%s' "$RELEASES" \
            | grep -oE "\"filename\":\"${TARBALL}\"[^}]*\"sha256\":\"[0-9a-f]+\"" \
            | grep -oE '[0-9a-f]{64}' | head -n1 || true)
    fi
    if [ -z "$EXPECTED_SHA" ]; then
        echo "Could not find official SHA256 for $TARBALL in Go release feed" >&2
        exit 1
    fi

    # Download to a temp file first; only remove the old Go after verifying.
    TMP=$(mktemp)
    trap 'rm -f "$TMP"' EXIT
    curl -fsSL "https://go.dev/dl/${TARBALL}" -o "$TMP"
    ACTUAL_SHA=$(sha256sum "$TMP" | awk '{print $1}')
    if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
        echo "Go tarball checksum mismatch for $TARBALL" >&2
        echo "  expected $EXPECTED_SHA" >&2
        echo "  got      $ACTUAL_SHA" >&2
        exit 1
    fi

    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$TMP"
    echo "Go upgraded to $GO_REQUIRED (sha256 verified)"
else
    echo "Go $GO_INSTALLED already matches go.mod"
fi
