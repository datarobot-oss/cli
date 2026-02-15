# Factory Configuration - DataRobot CLI

This directory contains configuration files for Factory integration with the DataRobot CLI project.

## Cloud Templates

Cloud Templates provide pre-configured development environments in the cloud for the DataRobot CLI.

### Available Setup Scripts

#### 1. **Full Setup** - `cloud-template-setup.sh`
Complete development environment with validation.

**Includes:**
- Task runner installation
- Development environment initialization
- Full linting and code quality checks
- CLI binary build
- Complete test suite with coverage

**Setup time:** ~3-5 minutes

**Use when:**
- Setting up a team reference environment
- Need validation of code quality before work
- Want full confidence in the environment

**To use:**
Copy the entire script contents when creating a Cloud Template in Factory settings.

#### 2. **Minimal Setup** - `cloud-template-setup-minimal.sh`
Fast setup with essentials only.

**Includes:**
- Task runner installation
- Development environment initialization
- CLI binary build

**Setup time:** ~1-2 minutes

**Use when:**
- You want to start coding quickly
- Plan to run tests/linters individually
- Prefer faster iteration cycles

**To use:**
Copy the entire script contents when creating a Cloud Template in Factory settings.

### Creating a Cloud Template

#### Step-by-Step

1. **Log into Factory** and navigate to **Settings → Cloud Templates**

2. **Click "Create Template"**

3. **Configure the Template:**
   - **Repository:** Enter the DataRobot CLI repo URL
   - **Template Name:** Choose a descriptive name (e.g., `dr-main`, `dr-dev`)
   - **Setup Script (Optional):** Copy the desired script from this directory

4. **Submit** - Factory clones the repo and runs the setup script

5. **Wait for "Ready" status** - Then you can use the template from any session

### Using a Cloud Template

#### From a Session

1. Start a Factory session
2. Click **Machine Connection** on the session start page
3. Select **Remote** tab
4. Choose your Cloud Template
5. Connect and start coding

#### Running Commands

Once connected, use the Terminal toolkit to run:

```bash
task run          # Run the CLI
task build        # Build the binary
task test         # Run tests
task lint         # Run linters
task run -- --help # Run CLI with arguments
```

### Understanding the Setup Scripts

Both scripts follow this pattern:

```bash
#!/usr/bin/env bash
set -euo pipefail
```

- `set -e` - Exit on any error
- `set -u` - Fail on undefined variables
- `set -o pipefail` - Fail if any command in a pipe fails

This ensures the setup is robust and fails fast on issues.

### Task Runner Commands

The setup scripts use [Task](https://taskfile.dev/) - a modern alternative to Make.

Key tasks (from `Taskfile.yaml`):

| Task | Purpose |
|------|---------|
| `task dev-init` | Initialize dev environment, install tools |
| `task lint` | Run all linters (formatting, go vet, golangci-lint, etc.) |
| `task build` | Build the CLI binary to `./dist/dr` |
| `task test` | Run full test suite with race detection |
| `task run` | Run the CLI directly |
| `task run -- <args>` | Run CLI with arguments |

### Customizing the Setup

To create a custom setup script for your team:

1. Edit `cloud-template-setup.sh` or `cloud-template-setup-minimal.sh`
2. Add or remove task commands as needed
3. Test locally: `bash .factory/cloud-template-setup.sh`
4. Use the updated script when creating a Cloud Template

Example: Skip linting to save time
```bash
# Comment out:
# echo "🧹 Running linters and formatters..."
# task lint
```

Example: Add custom setup
```bash
# Add after task build:
task test-coverage  # Generate HTML coverage report
```

### Troubleshooting

| Issue | Solution |
|-------|----------|
| **Setup fails with timeout** | Check the build logs. The full setup can take 3-5 min. If it exceeds 10 min, use the minimal script. |
| **Task not found after install** | The script installs to `.local/bin`. Ensure PATH is updated. Cloud environments typically have this in PATH. |
| **Permission denied errors** | The minimal setup uses `.local/bin` instead of `/usr/local/bin` to avoid permission issues. |
| **Out of disk space** | Cloud templates have ample storage. Check if Docker containers are not cleaned up from previous builds. |
| **Go version mismatch** | Cloud environments use the latest Go LTS. The CLI should be compatible with Go 1.22+. |

### Best Practices

1. **For teams:** Create one shared template (e.g., `dr-main`) from the main branch
2. **For experimentation:** Create templates per feature branch
3. **Share URLs:** Copy the template URL from Factory settings to share with teammates
4. **Version control:** Keep these scripts in `.factory/` so they're version controlled with the repo
5. **Iterate:** Test script changes locally before using them in templates

### Related Documentation

- **DataRobot CLI Project:** See `AGENTS.md` for coding guidelines and task documentation
- **Factory Cloud Templates:** https://docs.factory.ai/web/machine-connection/cloud-templates
- **Task Runner:** https://taskfile.dev/
- **Taskfile.yaml:** Full task definitions in the project root

### Next Steps

1. Choose a setup script (full or minimal)
2. Create your first Cloud Template following the step-by-step guide
3. Test it from a Factory session
4. Share with your team
5. Customize as needed for your workflow

