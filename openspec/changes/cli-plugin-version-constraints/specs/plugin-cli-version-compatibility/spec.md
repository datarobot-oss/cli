# Plugin CLI Version Compatibility Specification

## Purpose

Plugin manifests MAY declare `minCLIVersion` and `maxCLIVersion` semver bounds. The CLI MUST evaluate these bounds at plugin discovery, before a plugin is loaded, so an incompatible plugin never runs and instead surfaces a clear, actionable skip signal.

## Requirements

### Requirement: Manifest Version Bound Fields

The plugin manifest schema MUST support optional `minCLIVersion` and `maxCLIVersion` fields, each a plain semver string (no range/constraint-expression syntax). Either, both, or neither MAY be present.

#### Scenario: No bounds declared

- GIVEN a plugin manifest with neither `minCLIVersion` nor `maxCLIVersion` set
- WHEN the plugin is discovered
- THEN no compatibility check is performed
- AND the plugin loads

### Requirement: Bound Comparison Semantics

The system MUST treat `minCLIVersion` as an inclusive lower bound and `maxCLIVersion` as an inclusive upper bound. The system MUST compare only the core version (major.minor.patch) of the running CLI against declared bounds, ignoring any CLI prerelease identifier. The system MUST treat an unparseable running CLI version (including the literal `dev` build) as a bypass that loads the plugin unconditionally, and MUST treat a malformed declared bound as a skip condition, never as a pass-through load. The malformed-bound-skip rule MUST take precedence over the dev/unparseable-CLI-version bypass: a malformed declared bound always causes a skip, regardless of whether the running CLI version is `dev`, otherwise unparseable, or a normal parseable version. The dev/unparseable-CLI bypass MUST only apply when the declared bound(s) are well-formed.

#### Scenario: Min-only bound satisfied

- GIVEN a manifest declaring only `minCLIVersion: 1.0.0`
- WHEN the running CLI version is `1.0.0` or newer
- THEN the plugin loads

#### Scenario: Max-only bound satisfied, inclusive boundary

- GIVEN a manifest declaring only `maxCLIVersion: 2.3.0`
- WHEN the running CLI version is exactly `2.3.0`
- THEN the plugin loads
- WHEN the running CLI version is `2.3.1`
- THEN the plugin is skipped as incompatible

#### Scenario: Both bounds satisfied

- GIVEN a manifest declaring `minCLIVersion: 1.0.0` and `maxCLIVersion: 2.0.0`
- WHEN the running CLI version is `1.5.0`
- THEN the plugin loads

#### Scenario: Below minimum bound

- GIVEN a manifest declaring `minCLIVersion: 2.0.0`
- WHEN the running CLI version is `1.9.0`
- THEN the plugin is skipped as incompatible
- AND the skip identifies the CLI upgrade path

#### Scenario: Above maximum bound

- GIVEN a manifest declaring `maxCLIVersion: 1.5.0`
- WHEN the running CLI version is `1.6.0`
- THEN the plugin is skipped as incompatible

#### Scenario: Malformed declared bound

- GIVEN a manifest declaring an unparseable `minCLIVersion` or `maxCLIVersion` value
- WHEN the plugin is discovered
- THEN the plugin is skipped and reported
- AND the plugin MUST NOT load under any circumstance

#### Scenario: Unparseable or dev running CLI version

- GIVEN the running CLI version is `dev` or otherwise unparseable as semver
- AND a manifest declares any combination of well-formed bounds
- WHEN the plugin is discovered
- THEN the compatibility check is bypassed
- AND the plugin loads

#### Scenario: Dev running CLI version with a malformed declared bound

- GIVEN the running CLI version is `dev` or otherwise unparseable as semver
- AND a manifest declares a malformed `minCLIVersion` or `maxCLIVersion` value
- WHEN the plugin is discovered
- THEN the plugin is skipped and reported
- AND the malformed-bound skip takes precedence over the dev/unparseable-CLI bypass

#### Scenario: Prerelease running CLI version

- GIVEN a manifest declaring `maxCLIVersion: 1.2.0`
- WHEN the running CLI version is the prerelease `1.2.0-rc.1`
- THEN the comparison uses core version `1.2.0`
- AND the plugin loads because the core version satisfies the bound

### Requirement: Enforcement at All Discovery Sources

The compatibility check MUST run for every plugin discovery source: managed plugin loading, project-local plugin discovery, and PATH-based plugin discovery. No discovery source MAY bypass the check.

#### Scenario: Managed plugin below minimum

- GIVEN a managed plugin manifest declaring `minCLIVersion` above the running CLI version
- WHEN managed plugin loading runs
- THEN the plugin is skipped as incompatible

#### Scenario: Local or PATH plugin above maximum

- GIVEN a project-local or PATH-discovered plugin manifest declaring `maxCLIVersion` below the running CLI version
- WHEN discovery runs
- THEN the plugin is skipped as incompatible

### Requirement: Skip Ordering Relative to Name Deduplication

The compatibility check MUST run before a plugin name is reserved in discovery's deduplication logic. An incompatible plugin MUST NOT consume or block the name slot a compatible, identically-named plugin would otherwise claim, and MUST be reported as an incompatibility, not as a name conflict.

#### Scenario: Incompatible plugin does not block a compatible same-named plugin

- GIVEN two discovered manifests share the same plugin name
- AND the first manifest violates its declared CLI version bound
- AND the second manifest is within bounds (or declares none)
- WHEN discovery deduplicates by name
- THEN the first manifest is skipped and reported as incompatible, not as a name conflict
- AND the second manifest is registered under that name and loads

### Requirement: Structured, Single-Channel Skip Reporting

Version-incompatibility skips MUST be reported through the existing structured skip-record channel used for plugin conflicts, extended with a typed reason and detail, rather than through a second parallel reporting path.

#### Scenario: Skip carries a typed reason

- GIVEN a plugin is skipped for a bound violation
- WHEN the skip record is produced
- THEN the record's typed reason identifies it as a version-incompatibility skip, distinct from a name-conflict skip
- AND the record's detail names the violated bound and the running CLI version

### Requirement: User-Visible Feedback Scope

The system MUST log version-incompatibility skips at Info level without unsolicited output on ordinary command discovery that runs on every CLI invocation. The system MUST surface skipped plugins and their incompatibility reason through explicit plugin-listing and plugin-version-inspection commands. `dr plugin install`-time feedback for version bounds is out of scope for this capability.

#### Scenario: Silent on routine command discovery

- GIVEN an incompatible plugin exists among discovered plugins
- WHEN an unrelated `dr` command runs and triggers ordinary plugin discovery
- THEN no Info-level message is emitted to the user for that invocation beyond internal logging

#### Scenario: Visible via plugin listing

- GIVEN an incompatible plugin exists among discovered plugins
- WHEN the user runs the plugin-listing command
- THEN the incompatible plugin is absent from the loaded plugin list
- AND its incompatibility and required CLI version are visible in output
