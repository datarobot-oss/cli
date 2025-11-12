# Release Process

This document describes how to create and publish releases of the DataRobot CLI.

## Overview

The project uses [GoReleaser](https://goreleaser.com/) for automated releases. Releases are triggered by creating and pushing Git tags, which automatically builds binaries for multiple platforms and publishes them to GitHub.

## Prerequisites

- Write access to the repository
- All changes merged to the `main` branch
- Familiarity with [Semantic Versioning](https://semver.org/)

## Versioning

We follow [Semantic Versioning](https://semver.org/) (SemVer):

- **MAJOR.MINOR.PATCH** (e.g., `v1.2.3`)
- **Pre-releases**: `v1.2.3-rc.1`, `v1.2.3-beta.1`, `v1.2.3-alpha.1`

### Version Guidelines

**MAJOR** version when making incompatible API changes:

- Breaking changes to command-line interface
- Removing commands or flags
- Changing default behavior that breaks existing workflows

**MINOR** version when adding functionality in a backward-compatible manner:

- New commands or subcommands
- New flags or options
- New features

**PATCH** version when making backward-compatible bug fixes:

- Bug fixes
- Documentation updates
- Performance improvements

## Creating a Release

### Step 1: Ensure Main Branch is Ready

```bash
# Switch to main branch
git checkout main

# Pull latest changes
git pull origin main

# Verify all tests pass
task test

# Verify linting passes
task lint
```

### Step 2: Determine Next Version

Review recent changes and decide on the next version number based on SemVer guidelines above.

### Step 3: Create and Push Tag

```bash
# Create a new version tag
git tag v0.2.0

# Push the tag to trigger the release
git push origin v0.2.0
```

**Note:** The tag must start with `v` (e.g., `v1.0.0`, not `1.0.0`).

### Step 4: Monitor Release Process

1. Go to the [Actions tab](https://github.com/datarobot-oss/cli/actions) in GitHub
2. Watch the release workflow run
3. The workflow will:
   - Build binaries for multiple platforms (macOS, Linux, Windows)
   - Run tests
   - Generate release notes from commit messages
   - Create a GitHub release
   - Upload artifacts

### Step 5: Verify Release

Once the workflow completes:

1. Go to [Releases](https://github.com/datarobot-oss/cli/releases)
2. Verify the new release appears with:
   - Correct version number
   - Generated release notes
   - Binary artifacts for all platforms
   - Checksums file

### Step 6: Update Release Notes (Optional)

Edit the release notes on GitHub to:

- Add highlights of major changes
- Include upgrade instructions if needed
- Add breaking change warnings
- Include acknowledgments

## Pre-release Versions

For testing releases before making them generally available:

```bash
# Create a pre-release tag
git tag v0.2.0-rc.1

# Push the tag
git push origin v0.2.0-rc.1
```

Pre-release versions are marked as "Pre-release" on GitHub and can be used for testing.

## Testing the Release Process

To test the release process without publishing:

```bash
# Dry run (builds but doesn't publish)
goreleaser release --snapshot --clean

# Check output in dist/ directory
ls -la dist/
```

This creates build artifacts locally without creating a GitHub release.

## Rollback

If a release has issues:

### Delete the tag locally and remotely

```bash
# Delete local tag
git tag -d v0.2.0

# Delete remote tag
git push origin :refs/tags/v0.2.0
```

### Delete the GitHub release

- Go to Releases page
- Click on the problematic release
- Click "Delete this release"

### Fix the issues and create a new patch release

## Release Configuration

The release process is configured in `goreleaser.yaml`. Key configurations:

- **Builds**: Defines target platforms and architectures
- **Archives**: Creates distribution archives
- **Checksums**: Generates checksum files
- **Release notes**: Automatic generation from commits
- **Artifacts**: Files to include in the release

To validate the configuration:

```bash
goreleaser check
```

## Automated Release Workflow

The GitHub Actions workflow (`.github/workflows/release.yml`) automatically:

1. Triggers on tag push matching `v*`
2. Checks out the code
3. Sets up Go environment
4. Runs GoReleaser
5. Creates GitHub release
6. Uploads all artifacts

## Best Practices

1. **Always test before releasing:**
   - Run full test suite: `task test`
   - Run linters: `task lint`
   - Build locally: `task build`

2. **Use meaningful commit messages:**
   - They're used to generate release notes
   - Follow conventional commit format when possible

3. **Update CHANGELOG.md:**
   - Document significant changes
   - Include migration notes for breaking changes

4. **Communicate breaking changes:**
   - Update documentation
   - Add prominent notes in release description
   - Consider a major version bump

5. **Test installation:**
   - Test the install script after release
   - Verify binaries work on target platforms

## Troubleshooting

### Release workflow fails

- Check the Actions tab for error messages
- Verify `goreleaser.yaml` is valid: `goreleaser check`
- Ensure all required secrets are configured

### Tag already exists

```bash
# Delete and recreate if needed
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
git tag v0.2.0
git push origin v0.2.0
```

### Missing artifacts

- Verify build configuration in `goreleaser.yaml`
- Check build logs in GitHub Actions
- Test locally with `goreleaser release --snapshot --clean`

## Next Steps

- [Setup Guide](setup.md)&mdash;development environment setup
- [Building Guide](building.md)&mdash;detailed build information
- [Contributing](../../CONTRIBUTING.md)&mdash;contribution guidelines
