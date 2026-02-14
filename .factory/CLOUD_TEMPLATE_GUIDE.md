# Cloud Template Setup Guide - DataRobot CLI

This guide explains how to create and use a Cloud Template for the DataRobot CLI project.

## What is a Cloud Template?

A Cloud Template is a pre-configured, on-demand development environment that lives in the cloud. It provides:
- **Zero setup**: Open a session and start coding without local installs
- **Consistency**: Every teammate runs the exact same environment
- **Speed**: Heavy builds run on powerful cloud CPUs
- **Isolation**: Experiments in disposable templates keep your local machine clean
- **Collaboration**: Share template links for code reviews in live environments

## Creating a Cloud Template

### Step 1: Access Cloud Templates Settings

1. Log in to Factory
2. Click the **Settings** icon from the left sidebar
3. Select **Cloud Templates**

### Step 2: Create a New Template

1. Click **Create Template**
2. Enter the repository: `datarobot/cli` (or your fork URL)
3. Give your template a friendly name, e.g., `dr-main` or `dr-dev`
4. In the **Setup Script (Optional)** section, paste the contents of `.factory/cloud-template-setup.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "🚀 Setting up DataRobot CLI Cloud Template..."

# Install task runner if not already available
if ! command -v task &> /dev/null; then
    echo "📦 Installing Task runner..."
    sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
fi

# Initialize development environment and install tools
echo "🔧 Initializing development environment..."
task dev-init

# Run linting to ensure code quality
echo "🧹 Running linters and formatters..."
task lint

# Build the CLI binary
echo "🔨 Building CLI binary..."
task build

# Run tests with race detection and coverage
echo "✅ Running tests..."
task test

echo "✨ Cloud Template setup complete!"
```

5. (Optional) Add environment variables if needed
6. Click **Create**

Factory will clone the repo and run the setup script—this typically takes 3-5 minutes for the first build.

### Step 3: Wait for Template Ready Status

The new template appears in the Cloud Templates list with a status indicator. Once it shows **Ready**, you can use it.

## Using the Cloud Template

### Launch from a Session

1. Start a new Factory session
2. On the session start page, click **Machine Connection**
3. Select the **Remote** tab
4. Choose the Cloud Template you created (e.g., `dr-main`)
5. Factory attaches the template to your session

A green indicator appears in the top-right showing you're connected to the cloud template.

### Common Commands

Once connected, you can run commands via the Terminal toolkit:

```bash
# Run the CLI
task run

# Build a fresh binary
task build

# Run tests
task test

# Run linters
task lint

# Run the CLI with arguments
task run -- --help
```

## What the Setup Script Does

The setup script (`.factory/cloud-template-setup.sh`) performs these steps:

1. **Installs Task runner** - A task automation tool for Go projects
2. **Initializes dev environment** (`task dev-init`)
   - Downloads and installs Go linting tools
   - Installs testing dependencies (testify)
3. **Runs linters** (`task lint`)
   - `go mod tidy` - cleans dependencies
   - `go fmt` - formats code
   - `gofumpt` - advanced formatting
   - `go vet` - checks for suspicious code
   - `golangci-lint` - comprehensive linting
   - `goreleaser check` - validates release config
4. **Builds the binary** (`task build`)
   - Creates `./dist/dr` with proper version info
5. **Runs tests** (`task test`)
   - Full test suite with race detection and coverage

## Environment Variables

The setup script uses no external environment variables. All configuration comes from the repository's Taskfile and Go modules.

If you need custom environment variables for your template:
1. Add them in the "Environment Variables" section when creating the template
2. Reference them in any custom setup commands as `$VAR_NAME`

## Best Practices

### For Faster Setup

- Cloud Template setup includes full linting and testing (takes ~3-5 min)
- If you want faster iteration, create a minimal setup script that only installs dependencies:

```bash
#!/usr/bin/env bash
set -euo pipefail

task dev-init
task build
```

This skips linting and testing, reducing setup time to ~1-2 minutes.

### For Team Collaboration

1. **Share the template**: Copy the template URL and share with teammates
2. **Code reviews in context**: Reviewers can jump into the live environment with code and ports running
3. **Consistent environment**: Everyone on the team uses the exact same setup

### Troubleshooting Setup Issues

| Issue | Solution |
|-------|----------|
| "Setup script failed: ..." | Check build logs for specific error. Run the script locally to debug. |
| Command not found (e.g., `task`) | The install command in the setup script should handle this. Check build logs. |
| Slow build time | Remove `task lint` and `task test` from the script if not needed initially. Add them back later. |
| Out of disk space | Cloud templates have adequate storage. Check if the build is trying to cache unnecessarily. |
| Go version mismatch | The cloud environment uses the latest Go LTS version. Check `.github/workflows` for CI Go version. |

## Customizing Your Template

You can create multiple Cloud Templates with different setups:

- **`dr-main`**: Full setup with linting and tests (validation)
- **`dr-dev`**: Minimal setup (fast iteration)
- **`dr-release`**: Setup tailored for release workflows

Each template is independent and can have different setup scripts and environment variables.

## Next Steps

1. Create your first template following the steps above
2. Launch it from a session and verify commands work
3. Share the template with your team
4. Iterate on the setup script based on your workflow needs

For more information, see the [Factory Cloud Templates documentation](https://docs.factory.ai/web/machine-connection/cloud-templates).
