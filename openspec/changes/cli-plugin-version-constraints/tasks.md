# Tasks: Plugin manifest CLI version constraints (CFX-4730)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~400-430 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (mechanism, ~180 lines) → PR 2 (integration, ~230 lines) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Add `MaxCLIVersion`, `PluginSkipReason`, `Reason`/`Detail`, and inert `compatibleCLIVersion`/`cliVersionSkip` predicate + seam; no call sites wired | PR 1 | `go test ./internal/plugin/... -run 'CLIVersion|TestValidateManifests' -race` | N/A — pure predicate, no discovery behavior change yet | Revert `cli_version.go`, `cli_version_test.go`, and the `types.go`/`validation.go` diffs; discovery untouched |
| 2 | Wire `cliVersionSkip` into both discover.go call sites before `seen[...]`, level-branch `LogConflicts`/add `ConflictsForReason`, update `cmd/plugin/discovery.go`, docs | PR 2 | `go test ./internal/plugin/... ./cmd/plugin/... -race` | `dr plugin list` / `dr plugin version <name>` against a manifest with `maxCLIVersion` below current | Revert discover.go/discovery.go diffs; PR 1's inert predicate stays, plugins keep loading unconditionally |

## Phase 1: Foundation — Types (PR 1)

- [x] 1.1 In `internal/plugin/types.go`: add `MaxCLIVersion string` (`json:"maxCLIVersion,omitempty"`) to `PluginManifest`; add `PluginSkipReason` type with `SkipReasonNameConflict = iota` (zero value); add `Reason PluginSkipReason` and `Detail string` to `PluginConflict`. Spec: Manifest Version Bound Fields.
- [x] 1.2 Update `PluginConflict` doc comment to describe the extended reason/detail contract without changing existing zero-value literals' meaning.

## Phase 2: Foundation — Predicate (PR 1, TDD)

- [x] 2.1 RED: create `internal/plugin/cli_version_test.go` with table-driven `TestCompatibleCLIVersion` covering: no bounds; min satisfied/at-boundary/violated; max at-boundary (loads)/violated; both bounds; `1.2.0-rc.1` vs `max 1.2.0` loads; `dev`/garbage CLI bypass; malformed min/max → `(false, err)` including on `dev` CLI; `v`-prefixed bounds. Spec scenarios: Min-only, Max-only inclusive, Both bounds, Below minimum, Above maximum, Malformed declared bound, Unparseable/dev CLI, Dev CLI with malformed bound, Prerelease CLI.
- [x] 2.2 GREEN: implement `internal/plugin/cli_version.go` with package var `currentCLIVersion = version.Version` (test seam), `coreVersion(v *semver.Version) *semver.Version`, and `compatibleCLIVersion(cliVersion, minBound, maxBound string) (bool, error)` per design's ordering: both empty → true; parse bounds (error → false,err); parse cliVersion (error → true,nil bypass); compare core versions.
- [x] 2.3 GREEN: add `cliVersionSkip(manifest *PluginManifest, path string) *PluginConflict` returning `nil` or `&PluginConflict{Reason: SkipReasonVersionIncompatible, Detail: ...}` with exact wording from design (min/max/malformed templates).
- [x] 2.4 Run `go test ./internal/plugin/... -run CLIVersion -race -v`; confirm all Phase 2.1 cases pass.

## Phase 3: Validation Comments (PR 1)

- [x] 3.1 In `internal/plugin/validation.go` (L50, L109-111, L129): update comments to state `MaxCLIVersion` is also excluded from cross-manifest comparison alongside `MinCLIVersion` (field compare already omits it by construction — comment-only change, no `validateManifests` logic edit).

## Phase 4: Discovery Wiring (PR 2, TDD)

- [x] 4.1 RED: in `internal/plugin/discover_test.go`, extend `createManagedTestPlugin` to accept min/max bounds; add case asserting an out-of-range managed plugin is absent from results and present in conflicts with `Reason: SkipReasonVersionIncompatible`. Spec: Managed plugin below minimum.
- [x] 4.2 RED: add `DiscoverWithContextSuite` case — two PATH dirs export the same manifest name, lexicographically-first violates `maxCLIVersion`; assert second registers under that name, conflicts hold exactly one `SkipReasonVersionIncompatible` record and zero `SkipReasonNameConflict` records. Spec: Skip Ordering Relative to Name Deduplication.
- [x] 4.3 RED: override `currentCLIVersion` (with `t.Cleanup`/`defer` restore) in `TestDuplicatePATHEntryDoesNotWarn` and the XDG-config-dirs discovery test to a real semver so the `dev` bypass does not mask assertions.
- [x] 4.4 Confirmed 4.1-4.3 fail against current code (`go vet ./internal/plugin/...` → `undefined: ConflictsForReason` compile failure).
- [x] 4.5 GREEN: call `cliVersionSkip` in `loadManagedPlugin` (before the executable resolution and the `seen[...]` reservation) and `getManifestsParallel`'s merge loop (before its `seen[...]` reservation); append its non-nil result to conflicts and `continue` without setting `seen[name]`.
- [x] 4.6 `go test ./internal/plugin/... -race -v` — all new and existing cases GREEN.

## Phase 5: Reporting & CLI Wiring (PR 2, TDD)

- [x] 5.1 RED: extended `TestLogConflicts` asserting name-conflict wording/level is byte-identical to today (WARN, no INFO), added `TestLogConflictsVersionIncompatible` (INFO, includes `Detail`, no WARN), added `TestConflictsForReason`, and extended `TestConflictsForName` fixtures with a mixed-reason record proving the filter stays reason-agnostic. Spec: Structured Single-Channel Skip Reporting.
- [x] 5.2 GREEN: added `ConflictsForReason(conflicts []PluginConflict, reason PluginSkipReason) []PluginConflict` in `internal/plugin/discover.go`; `LogConflicts` now switches on `Reason` — Warn for `SkipReasonNameConflict`, Info for `SkipReasonVersionIncompatible` (exhaustive switch, no `default`, to satisfy the `exhaustive` linter).
- [x] 5.3 GREEN: extracted `reportDiscoveryConflicts` in `cmd/plugin/discovery.go`, called from `RegisterPluginCommands` in place of the direct `LogConflicts(conflicts)` call; it calls `LogConflicts(ConflictsForReason(conflicts, SkipReasonNameConflict))` and logs a Debug line for the remainder, so routine discovery stays silent for version skips. Added `TestReportDiscoveryConflicts` (RED confirmed via `undefined: reportDiscoveryConflicts` before implementation) covering name-conflict-only, version-only, and mixed cases. Spec: Silent on routine command discovery.
- [x] 5.4 Verified via a real built binary (`go build -ldflags "-X .../version.Version=1.0.0"`) with a fake `dr-oldwidget` PATH plugin declaring `maxCLIVersion: 0.9.0`: `dr plugin list` excludes it from the table and prints `INFO Plugin skipped: CLI version incompatible ... detail="supports dr <= 0.9.0 (running 1.0.0); update the plugin"`; `dr plugin version oldwidget` prints the same Info line then `Error: plugin "oldwidget" not found`; plain `dr --help` prints neither the plugin name nor "version incompatible" (confirms silence on ordinary discovery). No code changes were needed in `cmd/plugin/list/cmd.go` or `cmd/plugin/version/cmd.go` — confirmed empirically, not just assumed from design.

## Phase 6: Documentation (PR 2)

- [x] 6.1 `docs/development/plugins.md` (~L82-89): added `minCLIVersion`/`maxCLIVersion` to the manifest JSON example and a new Optional-fields bullet documenting inclusive bounds, core-version-only comparison, malformed-bound-always-skips, the `dev`/unparseable-CLI bypass, and the reporting channel.
- [x] 6.2 `docs/development/remote-plugins.md` (~L61): added `maxCLIVersion` next to the existing `minCLIVersion` example, with a cross-reference to the full semantics in `plugins.md`.

## Phase 7: Final Gates (PR 1 and PR 2, each before opening)

- [x] 7.1 `task lint` clean on each slice. (PR 1 done; PR 2 done — 0 issues on all 3 GOOS targets)
- [x] 7.2 `task test` (race + coverage) green on each slice. (PR 1 done; PR 2 done — full repo suite green, `internal/plugin` 80.9% coverage)

## Delivery Status

PR 1: https://github.com/datarobot-oss/cli/pull/814 (draft, branch `aj/cli-version-constraints` off `main`). All PR-1-scoped tasks (Phase 1-3, and the PR-1 portion of Phase 7) complete.

PR 2: branch `aj/cli-version-constraints-discovery` off `aj/cli-version-constraints` (stacked). All PR-2-scoped tasks (Phase 4-6, and the PR-2 portion of Phase 7) complete. See apply-progress for the PR URL once opened.
