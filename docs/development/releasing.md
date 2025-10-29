# Release process

This page describes how to create and publish releases of the DataRobot CLI.

## Overview

This project uses [GoReleaser](https://goreleaser.com/) for automated releases. Trigger releases by creating and pushing Git tags, which automatically build binaries for multiple platforms and publish them to GitHub.

## Prerequisites

- Write access to the repository.
- All changes are merged to the `main` branch.
- Familiarity with [semantic versioning](https://semver.org/).

## Versioning

Versioning follows [semantic versioning](https://semver.org/) conventions (SemVer).

- **MAJOR.MINOR.PATCH** (e.g., `v1.2.3`)
- **Pre-releases**: `v1.2.3-rc.1`, `v1.2.3-beta.1`, `v1.2.3-alpha.1`

### Version guidelines

Use **MAJOR** version when making incompatible API changes, including:

- Breaking changes to the command-line interface
- Removing commands or flags
- Changing default behavior that breaks existing workflows

Use **MINOR** version when adding functionality in a backward-compatible manner, including:

- New commands or subcommands
- New flags or options
- New features

Use **PATCH** version when making backward-compatible bug fixes, including:

- Bug fixes
- Documentation updates
- Performance improvements

## Create a release

### 1. Ensure the main branch is ready

```bash
# Switch to the main branch
git checkout main

# Pull the latest changes
git pull origin main

# Verify that all tests pass
task test

# Verify that linting passes
task lint
```

### 2. Determine the next version

Review any recent changes and decide on the next version number based on the [semantic versioning](#versioning) guidelines.

### 3. Create and push a tag

When creating a tag, note that it must start with `v` (e.g., `v1.0.0`, not `1.0.0`).

```bash
# Create a new version tag
git tag v0.2.0

# Push the tag to trigger the release
git push origin v0.2.0
```

### 4. Monitor the release process

1. Go to the [Actions tab](https://github.com/datarobot-oss/cli/actions) in GitHub.
2. Watch the release workflow run.
3. The workflow will:
   - Build binaries for multiple platforms (macOS, Linux, Windows)
   - Run tests
   - Generate release notes from commit messages
   - Create a GitHub release
   - Upload artifacts

### 5. Verify the release

Once the workflow completes:

1. Go to [Releases](https://github.com/datarobot-oss/cli/releases).
2. Verify the new release appears with:
   - The correct version number
   - Generated release notes
   - Binary artifacts for all platforms
   - A checksums file

### 6. Update release notes

Optional. Edit the release notes on GitHub to:

- Add highlights of major changes
- Include any necessary upgrade instructions
- Add breaking change warnings
- Include acknowledgments

## Pre-release versions

To test releases before making them generally available, use the following commands.

```bash
# Create a pre-release tag
git tag v0.2.0-rc.1

# Push the tag
git push origin v0.2.0-rc.1
```

Pre-release versions are marked as "Pre-release" on GitHub and can be used for testing.

## Test the release process

To test the release process without publishing, use the commands below. They create build artifacts locally without creating a GitHub release.

```bash
# Dry run (builds but doesn't publish)
goreleaser release --snapshot --clean --skip=sign

# Check output in the dist/ directory
ls -la dist/
```

## Rollback

If a release has issues, the following actions are available.

### Delete the tag locally and remotely

```bash
# Delete a local tag
git tag -d v0.2.0

# Delete a remote tag
git push origin :refs/tags/v0.2.0
```

### Delete the GitHub release

- Go to the **Releases** page.
- Click on the problematic release.
- Click **Delete this release**.

### Fix the issues and create a new patch release

## Release configuration

The release process is configured in `goreleaser.yaml`. Key configurations:

- **Builds**: Defines target platforms and architectures.
- **Archives**: Creates distribution archives.
- **Checksums**: Generates checksum files.
- **Release notes**: Automatic generation from commits.
- **Artifacts**: Files to include in the release.

To validate the configuration:

```bash
goreleaser check
```

## Automated release workflow

The GitHub Actions workflow (`.github/workflows/release.yml`) automatically:

1. Triggers on tag push matching `v*`
2. Checks out the code
3. Sets up Go environment
4. Runs GoReleaser
5. Creates GitHub release
6. Uploads all artifacts

## Best practices

1. Always test before releasing.
   - Run full test suite: `task test`
   - Run linters: `task lint`
   - Build locally: `task build`

2. Use meaningful commit messages.
   - Commit messages are used to generate release notes
   - Follow the conventional commit format when possible

3. Update `CHANGELOG.md`.
   - Document significant changes
   - Include migration notes for breaking changes

4. Communicate breaking changes.
   - Update documentation
   - Add prominent notes in the release description
   - Consider a major version bump

5. Test the installation.
   - Test the install script after the release
   - Verify that binaries work on target platforms

## Troubleshooting

### Release workflow fails

- Check the **Actions** tab for error messages.
- Verify that`goreleaser.yaml` is valid: `goreleaser check`.
- Ensure all required secrets are configured.

### Tag already exists

```bash
# Delete and recreate the tag if needed
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
git tag v0.2.0
git push origin v0.2.0
```

### Missing artifacts

- Verify the build configuration in `goreleaser.yaml`.
- Check the build logs in GitHub Actions.
- Test locally with `goreleaser release --snapshot --clean`.

## Next steps

- [Setup Guide](setup.md): Outlines development environment setup.
- [Building Guide](building.md): Provides detailed build information.
- [Contributing](../../CONTRIBUTING.md): Contribution guidelines.
