#!/bin/bash
# Package a plugin directory into a .tar.xz archive
# Usage: ./scripts/package-plugin.sh <plugin-dir> <output-name> [version]

set -e

PLUGIN_DIR="$1"
OUTPUT_NAME="$2"
VERSION="${3:-1.0.0}"

if [[ -z "$PLUGIN_DIR" || -z "$OUTPUT_NAME" ]]; then
    echo "Usage: $0 <plugin-dir> <output-name> [version]"
    echo "Example: $0 docs/plugins/dr-apps dr-apps 1.0.0"
    exit 1
fi

if [[ ! -d "$PLUGIN_DIR" ]]; then
    echo "Error: Plugin directory '$PLUGIN_DIR' does not exist"
    exit 1
fi

if [[ ! -f "$PLUGIN_DIR/manifest.json" ]]; then
    echo "Error: manifest.json not found in '$PLUGIN_DIR'"
    exit 1
fi

OUTPUT_FILE="${OUTPUT_NAME}-${VERSION}.tar.xz"
OUTPUT_DIR="docs/plugins/${OUTPUT_NAME}"
OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_FILE}"

# Create plugin directory
mkdir -p "$OUTPUT_DIR"

echo "Packaging plugin..."
echo "  Source: $PLUGIN_DIR"
echo "  Output: $OUTPUT_PATH"
echo "  Version: $VERSION"

# Create tar.xz archive (package contents without the outer directory)
tar -cJf "$OUTPUT_PATH" -C "$PLUGIN_DIR" .

# Calculate SHA256
SHA256=$(shasum -a 256 "$OUTPUT_PATH" | awk '{print $1}')

echo ""
echo "Package created: $OUTPUT_PATH"
echo "SHA256: $SHA256"
echo ""
echo "Add to index.json:"{OUTPUT_NAME}/${VERSION}/${OUTPUT_FILE}
cat <<EOF
{
  "version": "$VERSION",
  "url": "https://cli.datarobot.com/plugins/$OUTPUT_FILE",
  "sha256": "$SHA256"
}
EOF
