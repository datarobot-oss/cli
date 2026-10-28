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

- [ ] 4.1 RED: in `internal/plugin/discover_test.go`, extend `createManagedTestPlugin` to accept min/max bounds; add case asserting an out-of-range managed plugin is absent from results and present in conflicts with `Reason: SkipReasonVersionIncompatible`. Spec: Managed plugin below minimum.
- [ ] 4.2 RED: add `DiscoverWithContextSuite` case — two PATH dirs export the same manifest name, lexicographically-first violates `maxCLIVersion`; assert second registers under that name, conflicts hold exactly one `SkipReasonVersionIncompatible` record and zero `SkipReasonNameConflict` records. Spec: Skip Ordering Relative to Name Deduplication.
- [ ] 4.3 RED: override `currentCLIVersion` (with `t.Cleanup` restore) in `TestDuplicatePATHEntryDoesNotWarn` and the XDG-config-dirs discovery test to a real semver so the `dev` bypass does not mask assertions.
- [ ] 4.4 Confirm 4.1-4.3 fail against current code (`go test ./internal/plugin/... -race`).
- [ ] 4.5 GREEN: call `cliVersionSkip` in `loadManagedPlugin` (~L304) and `getManifestsParallel` (~L441 loop), both immediately before the `seen[...]` reservation; append its non-nil result to conflicts and `continue` without setting `seen[name]`.
- [ ] 4.6 Run `go test ./internal/plugin/... -race -v` until 4.1-4.3 are GREEN.

## Phase 5: Reporting & CLI Wiring (PR 2, TDD)

- [ ] 5.1 RED: extend `TestLogConflicts` asserting name-conflict wording is byte-identical to today, and a `SkipReasonVersionIncompatible` record emits Info-level output including `Detail`. Add `TestConflictsForReason`; extend `TestConflictsForName` fixtures with mixed reasons proving the filter stays reason-agnostic. Spec: Structured Single-Channel Skip Reporting.
- [ ] 5.2 GREEN: add `ConflictsForReason(conflicts []PluginConflict, reason PluginSkipReason) []PluginConflict` in `internal/plugin/discover.go`; change `LogConflicts` to switch on `Reason` — Warn for name conflicts (unchanged wording), Info for version incompatibility.
- [ ] 5.3 GREEN: in `cmd/plugin/discovery.go` (L56), change `RegisterPluginCommands` to call `LogConflicts(ConflictsForReason(conflicts, SkipReasonNameConflict))` and add a Debug line for the remainder, so routine discovery stays silent for version skips. Spec: Silent on routine command discovery.
- [ ] 5.4 Verify `dr plugin list` / `dr plugin version <name>` surface skipped/incompatible plugins with required-version detail (manual harness run, or existing list/version command tests if present).

## Phase 6: Documentation (PR 2)

- [ ] 6.1 `docs/development/plugins.md` (~L82-89): add `maxCLIVersion` and `minCLIVersion` to the Optional fields list with inclusive-bound and dev-bypass semantics.
- [ ] 6.2 `docs/development/remote-plugins.md` (~L61): add `maxCLIVersion` next to the existing `minCLIVersion` example.

## Phase 7: Final Gates (PR 1 and PR 2, each before opening)

- [x] 7.1 `task lint` clean on each slice. (PR 1 done; PR 2 pending)
- [x] 7.2 `task test` (race + coverage) green on each slice. (PR 1 done; PR 2 pending)
