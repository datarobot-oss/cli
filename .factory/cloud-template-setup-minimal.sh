#!/usr/bin/env bash
set -euo pipefail

echo "🚀 Setting up DataRobot CLI Cloud Template (minimal)..."

# Install task runner if not already available
if ! command -v task &> /dev/null; then
    echo "📦 Installing Task runner..."
    TASK_INSTALL_DIR="${TASK_INSTALL_DIR:-.local/bin}"
    mkdir -p "$TASK_INSTALL_DIR"
    sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b "$TASK_INSTALL_DIR"
    export PATH="$TASK_INSTALL_DIR:$PATH"
fi

# Initialize development environment and install tools
echo "🔧 Initializing development environment..."
task dev-init

# Build the CLI binary
echo "🔨 Building CLI binary..."
task build

echo "✨ Cloud Template setup complete!"
echo "📝 To use your environment, run:"
echo "   - task run          (run the CLI)"
echo "   - task build        (rebuild the binary)"
echo "   - task test         (run tests)"
echo "   - task lint         (run linters)"
