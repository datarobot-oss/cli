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
goreleaser release --snapshot --clean

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

The GitHub Actions workflow (`.github/workflows/release.yaml`) automatically:

1. Triggers on tag push matching `v*`
2. Checks out the code
3. Sets up Go environment
4. Runs GoReleaser
5. Creates GitHub release
6. Uploads all artifacts
7. Verifies installation on all platforms (Linux, macOS, Windows)
8. Sends Slack notification on success

### Post-release verification

After GoReleaser creates the release, the workflow follows a "verify before promoting" approach:

1. **Mark as pre-release**: The release is immediately marked as a pre-release after creation
2. **Verify installation**: The workflow tests installation on all supported platforms (Linux, macOS, Windows)
3. **Promote on success**: If verification passes, stable releases are recreated without pre-release status and a success Slack notification is sent
4. **Stay as pre-release on failure**: If verification fails, the release remains marked as a pre-release and a warning Slack notification is sent

This approach ensures users installing "latest" never get a broken release. Semantic pre-releases (e.g., `v1.0.0-rc.1`) remain as pre-releases even after successful verification.

To fix a failed release:
1. Investigate the installation failure in the workflow logs
2. Fix the underlying issue (usually missing or corrupted binaries)
3. Either re-upload the binaries and manually remove pre-release status via GitHub UI, or delete and recreate the release

### Testing installation manually

Use the **Installation Tests (On Demand)** workflow to test installation without creating a release:

1. Go to **Actions** > **Installation Tests (On Demand)**
2. Click **Run workflow**
3. Enter a version (e.g., `v0.2.49`) or leave empty for latest
4. Click **Run workflow**

This is useful for verifying installation scripts work correctly before or after releases.

## Best practices

1. Never tag the same commit twice.
   - Each semver tag must point to a unique commit.
   - If you need to release a new version, ensure the commit differs from any previously tagged commit (e.g., bump the changelog or add a fixup commit before tagging).
   - Creating two tags on the same commit causes ambiguous version resolution and should be avoided regardless of whether the tags are stable or pre-release.

2. Always test before releasing.
   - Run full test suite: `task test`
   - Run linters: `task lint`
   - Build locally: `task build`

3. Use meaningful commit messages.
   - Commit messages are used to generate release notes
   - Follow the conventional commit format when possible

4. Update `CHANGELOG.md`.
   - Document significant changes
   - Include migration notes for breaking changes

5. Communicate breaking changes.
   - Update documentation
   - Add prominent notes in the release description
   - Consider a major version bump

6. Test the installation.
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
- [Contributing guide](https://github.com/datarobot-oss/cli/blob/main/CONTRIBUTING.md)&mdash;contribution guidelines.
