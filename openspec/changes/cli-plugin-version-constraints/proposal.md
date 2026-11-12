# Proposal: Plugin manifest CLI version constraints (CFX-4730)

## Intent

A plugin can require a newer `dr` than the user runs. Today `PluginManifest.MinCLIVersion` exists but is never enforced, so an incompatible plugin loads and fails later with a confusing error. Users get no signal that the fix is upgrading the CLI.

## Scope

### In Scope

- Enforce `minCLIVersion` and add `maxCLIVersion` (both plain semver, inclusive bounds) in `internal/plugin/types.go`.
- Compatibility check at discovery for all three sources: `loadManagedPlugin` (managed) and `getManifestsParallel` (local + PATH).
- Report skips as structured data on the existing skip channel (`PluginConflict` gains a typed `Reason` + `Detail`), logged Info by `LogConflicts`; min-violations name `dr self update`.
- Docs: `docs/development/plugins.md`, `docs/development/remote-plugins.md`.

### Out of Scope

- `versions.yaml` / template-declared floors (Approach B, already shipped).
- Blocking or warning inside `dr plugin install` (see Q4).
- Constraint-expression syntax (`^1.2.0`, ranges); addable later as a new field.

## Capabilities

### New Capabilities

- `plugin-cli-version-compatibility`: manifest-declared CLI version bounds, evaluated at plugin discovery, with user-visible skip feedback.

### Modified Capabilities

- None (`openspec/specs/` does not exist yet).

## Approach

`compatibleCLIVersion(cliVersion, min, max)` using `Masterminds/semver/v3` (existing dependency). Rules:

| Case | Behavior |
|---|---|
| No bounds declared | Load |
| CLI version `dev` or unparseable | Load (bypass; our fault, not the plugin's) |
| Unparseable bound | **Skip + report** — never fall through and load (PR #357 defect) |
| Out of bounds | Skip + report |
| Prerelease CLI (`1.2.0-rc.1`) | Compared on core version |

Checked **before** `seen[name]` reservation, so an incompatible plugin never burns a name a compatible same-named plugin could claim, and is reported as incompatible, not as a name conflict.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/plugin/types.go` | Modified | `MaxCLIVersion`; `Reason`/`Detail` on skip record |
| `internal/plugin/discover.go` | Modified | New check + 2 call sites; `LogConflicts` switches on reason |
| `internal/plugin/validation.go` | Modified | Exclude `MaxCLIVersion` from cross-validation |
| `docs/development/*plugins.md` | Modified | Document both fields |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Plugin disappears with no obvious cause | Med | Info-level message names the constraint and the CLI version |
| Ordering/dedup regression in parallel path | Med | Check before name reservation; extend `discover_test.go` |
| Renaming to `cliVersion` would break shipped manifests | — | Rejected: keep `minCLIVersion` |

## Rollback Plan

Revert the single commit/PR. The fields become inert data again (their pre-change state); no migration, no persisted state.

## Dependencies

None new — `Masterminds/semver/v3` already vendored.

## Success Criteria

- [ ] Both fields parsed and enforced for managed, local, and PATH plugins.
- [ ] Incompatible plugin skipped with an Info message, absent from `dr plugin list`.
- [ ] Malformed constraint skips, never loads.
- [ ] `dev` builds load every plugin.
- [ ] `task lint` and `task test` pass.

## Open Questions

Q1 inclusive vs exclusive `maxCLIVersion`; Q2 prerelease semantics; Q3 skip channel extension vs new type; Q4 `dr plugin install` feedback. See proposal question round.

## Implementation Note

Delivery lands as a fresh branch or resumed PR #357 — decided in tasks/apply, not here. Budget: 400 review lines, auto-chain.
