# Design: Plugin manifest CLI version constraints (CFX-4730)

## Technical Approach

One pure predicate (`compatibleCLIVersion`) plus one record-producing wrapper (`cliVersionSkip`), called from exactly two places in `internal/plugin/discover.go`, both immediately **before** the `seen[...]` name reservation. No function signature changes anywhere: `loadManagedPlugin` already returns `*PluginConflict`, and `getManifestsParallel` already returns `[]PluginConflict`. `PluginConflict` gains a typed `Reason` (zero value = name conflict) and a `Detail` string, so the single existing skip channel carries both kinds and every current call site keeps compiling.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Skip channel | Extend `PluginConflict` with `Reason`+`Detail` | Second `[]PluginIncompatibility` return | A 4th return value churns 8 discovery funcs, the registry struct and 3 `cmd/` sites against a 400-line budget; `ConflictsForName` keeps working unchanged. |
| Reason zero value | `SkipReasonNameConflict = iota` | Explicit `SkipReasonUnknown` first | Every existing `PluginConflict{Name:…, Path:…}` literal in `discover.go` and `discover_test.go` keeps its exact meaning — near-zero diff. |
| Comparison basis | Core version only, on **both** sides | Raw `Compare` | Masterminds orders `1.2.0-rc.1 < 1.2.0`, so an rc build would fail its own `maxCLIVersion: 1.2.0`. Stripping prerelease on CLI *and* bounds makes the relation symmetric and matches the spec. |
| Malformed bound vs `dev` bypass | Parse and reject bounds **before** the unparseable-CLI bypass | Bypass first | Spec: a malformed bound "MUST NOT load under any circumstance". Parsing first also gives plugin authors — who work on `dev` builds — immediate feedback on a manifest typo. Narrows the dev-bypass scenario to *valid* bounds. |
| Check placement | Sequential merge loop of `getManifestsParallel`, not the goroutine | Inside the fetch goroutine | Keeps the deterministic lexicographic "first binary wins" ordering the merge loop guarantees; no new shared state. |
| Routine-discovery noise | `RegisterPluginCommands` logs only name conflicts at Warn | Log everything | Spec forbids per-invocation Info output; `dr plugin list` / `dr plugin version` surface the Info line. |
| Test seam | Package-level `var currentCLIVersion = version.Version` | Read `version.Version` inline | `version.Version` is `"dev"` in tests, which would bypass the check and make every discovery test vacuous. |

## Data Flow

    manifest ──→ cliVersionSkip(manifest, path)
                      │ nil  ──→ seen[name]=true ──→ DiscoveredPlugin
                      └ skip ──→ conflicts (Reason=VersionIncompatible)  ← name NOT reserved
    conflicts ──→ LogConflicts ──→ Warn (name conflict) | Info (version) → stderr

`discoverPathDirsParallel`'s cross-dir merge needs **no** third call site: incompatible plugins never leave `getManifestsParallel`, so they never enter `crossDirSeen`.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/plugin/cli_version.go` | Create | `compatibleCLIVersion`, `coreVersion`, `cliVersionSkip`, `currentCLIVersion` seam |
| `internal/plugin/types.go` | Modify | `MaxCLIVersion` field; `PluginSkipReason`; `Reason`+`Detail` on `PluginConflict` |
| `internal/plugin/discover.go` | Modify | 2 call sites (`loadManagedPlugin` L304, `getManifestsParallel` L441 — both before `seen[…]`); `LogConflicts` switch; new `ConflictsForReason` |
| `cmd/plugin/discovery.go` | Modify | L56: `LogConflicts(ConflictsForReason(conflicts, SkipReasonNameConflict))` + one Debug line for the rest |
| `internal/plugin/validation.go` | Modify | Comments L50/L109-111/L129: `MaxCLIVersion` also excluded from cross-manifest comparison (field-by-field compare already excludes it by omission — comments only) |
| `internal/plugin/cli_version_test.go` | Create | Table-driven predicate tests |
| `internal/plugin/discover_test.go` | Modify | Extend conflict/dedup tests (below) |
| `docs/development/plugins.md` | Modify | Optional-fields list (~L82-89) gains both fields |
| `docs/development/remote-plugins.md` | Modify | Manifest example (~L61) gains `maxCLIVersion` |

`cmd/plugin/list/cmd.go` and `cmd/plugin/version/cmd.go` need **no** change: they pass the full/filtered list to `LogConflicts`, which now picks the level. `ExecPluginManifest` uses `DisallowUnknownFields`, so adding `MaxCLIVersion` to the struct is *required* for install-time validation of manifests that emit it.

## Interfaces / Contracts

```go
// compatibleCLIVersion reports whether cliVersion satisfies the inclusive
// [minBound, maxBound] range. Empty bounds are unconstrained. Comparison uses
// core versions only, so a prerelease CLI is judged by its major.minor.patch.
// An unparseable cliVersion (including the default "dev" build) is a bypass.
// A non-nil error means a bound is malformed; callers MUST skip, never load.
func compatibleCLIVersion(cliVersion, minBound, maxBound string) (bool, error)

// cliVersionSkip returns nil when the plugin may load, or a populated
// PluginConflict{Reason: SkipReasonVersionIncompatible} describing the breach.
func cliVersionSkip(manifest *PluginManifest, path string) *PluginConflict

type PluginSkipReason int
const (
    SkipReasonNameConflict PluginSkipReason = iota // zero value: pre-existing behavior
    SkipReasonVersionIncompatible
)
```

Order inside `compatibleCLIVersion`: both bounds empty → `true`; parse `minBound`/`maxBound` (error → `false, err`); parse `cliVersion` (error → `true, nil`); `core(cli).LessThan(core(min))` → false; `core(cli).GreaterThan(core(max))` → false. `coreVersion` = `semver.New(v.Major(), v.Minor(), v.Patch(), "", "")`. Bounds parse with lenient `semver.NewVersion`, matching the `^v?\d+\.\d+\.\d+` format already asserted in `integration_test.go:92`.

`Detail` wording: min → `requires dr >= 2.0.0 (running 1.9.0); run 'dr self update'`; max → `supports dr <= 1.5.0 (running 1.6.0); update the plugin`; malformed → `invalid maxCLIVersion "1.x"`.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | `compatibleCLIVersion` | Table: no bounds; min satisfied/at-boundary/violated; max at-boundary (loads) / violated; both bounds; `1.2.0-rc.1` vs `max 1.2.0` → loads; `dev` and garbage CLI → bypass; malformed min and max → `(false, err)` **including on a `dev` CLI**; `v`-prefixed bounds |
| Unit | `LogConflicts` | **Extend** `TestLogConflicts`: assert the name-conflict wording is byte-identical to today *and* a version record emits Info wording + `Detail`. Add `TestConflictsForReason`; extend `TestConflictsForName` fixtures with mixed reasons to prove the filter stays reason-agnostic |
| Integration | Ordering guarantee | **New case in `DiscoverWithContextSuite`**: two PATH dirs export the same manifest name; the lexicographically-first violates `maxCLIVersion`. Assert the second is registered under that name, conflicts contain exactly one `SkipReasonVersionIncompatible` record for the first path, and **zero** `SkipReasonNameConflict` records |
| Integration | Managed + local sources | Extend `createManagedTestPlugin` to accept bounds; assert an out-of-range managed plugin is absent from results and present in conflicts. Existing `TestDuplicatePATHEntryDoesNotWarn` and `FindsPluginsInXDGConfigDirs` must stay green with `currentCLIVersion` overridden to a real semver |

All discovery-level tests must set `currentCLIVersion` (with `t.Cleanup` restore); otherwise the `dev` bypass makes them assert nothing.

## Threat Matrix

| Boundary | Applicability |
|---|---|
| Documentation-like paths | N/A — no file-type classification changes; discovery still keys on the `dr-` prefix and `exec.LookPath`. |
| Git repository selection | N/A — no VCS interaction. |
| Commit state | N/A. |
| Push state | N/A. |
| PR commands | N/A. |

The one real boundary — which external executables become `dr` subcommands — is narrowed, never widened: the change can only *remove* a plugin from registration. Covered by the integration rows above.

## Migration / Rollout

No migration. Both fields are optional and inert until a manifest declares them. Rollback = revert the PR(s).

## Delivery Forecast

Estimated authored change: ~150-170 lines of production code and docs, ~220-260 lines of tests → **~400-430 changed lines, at or slightly over the 400 budget**. Under `auto-chain`, deliver as 2 slices: (1) `types.go` + `cli_version.go` + `cli_version_test.go` + `validation.go` comments (~180 lines, self-contained, no behavior change); (2) the two discovery call sites + `LogConflicts`/`ConflictsForReason` + `cmd/plugin/discovery.go` + discovery tests + docs (~230 lines). Slice 1 is inert-but-tested; slice 2 activates it.

## Open Questions

- [ ] None blocking. Noted for review: parsing bounds before the `dev` bypass means a malformed bound is reported even on a `dev` build — deliberate, per the spec's unconditional "MUST NOT load".
