# DataRobot CLI

The DataRobot command line interface to manage custom applications.

## Getting Started

### Prerequisites

First, install the required development tools:

```bash
task install-tools
```

### Building and Running

To build and start the CLI:

```bash
task build && ./dist/dr
```

For interactive template setup, use:

```bash
./dist/dr templates setup
```

## Release

In order to release a new version of the DR CLI, you will need to do the following:

- Merge all change you want to see in the new version
- Check the latest version and increment it according to [semantic versioning](https://semver.org/). For example,
    -  If the latest version was `v0.0.1`, the next one may be `v0.0.2` or `v0.0.2-rc.1` (if you want a dev release for pre-release checks).
    - If the latest version was `v0.0.1-rc.1`, the next one may be `v0.0.1` or `v0.0.1-rc.1` (if you want a dev release for pre-release checks).
- Create a new tag with the corresponding next version like:

```bash
git tag v0.0.2-rc.1
git push --tags
```

The release automation will take care of the rest.
It reacts to any tag that starts from `v.*` on any branch which allows to test release process without merging changes.