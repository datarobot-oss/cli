# Spec: Manifest caching for plugin discovery

> Status: approved design, deferred for later implementation (2026-09-24).
> Context: `dr self version --short` took ~1s; root cause was plugin discovery
> executing a slow pyenv shim (`dr-mongo-migrations`, ~570ms) on every invocation.
> User chose manifest caching over lazy discovery. The pyenv shim was deleted
> locally (70–120ms now), but discovery still execs every `dr-*` binary on PATH
> on every run. This spec makes that cheap.

**Goal:** make plugin discovery cheap on every invocation by caching `dr-* --manifest`
exec results, so warm runs (like `dr self version --short`) skip spawning plugin
binaries entirely. UX semantics unchanged: discovery still runs, help still lists
plugins.

## Where it goes

- **`internal/plugin/discover.go`** — `getManifestsParallel` / `getManifest` currently
  exec every `dr-*` binary found in PATH dirs (500ms timeout each). This is the
  ~500ms cost per invocation.
- **New file `internal/plugin/manifest_cache.go`** — the cache.
- **Cache file:** `plugin-manifest-cache.json` in the config dir, stored via the
  existing `internal/state` helpers (same home as `state.yaml`). Load on discovery
  start, save once at the end.

## Cache design

```go
// manifestCache is a single JSON file mapping executable path -> entry.
type cacheEntry struct {
    Manifest  *PluginManifest `json:"manifest,omitempty"` // nil for failed fetches
    Failed    bool            `json:"failed,omitempty"`
    ModTime   int64           `json:"mtime"`   // unix nanos, invalidation key
    Size      int64           `json:"size"`
    CachedAt  time.Time       `json:"cached_at"`
}
```

- **Key:** executable absolute path. **Invalidation:** `mtime + size` of the binary,
  plus an age cap.
- **Success entries:** used when mtime+size still match (binary unchanged) and
  age < 24h. The age cap only guards symlinked shims (e.g. pyenv shims whose mtime
  doesn't change when the underlying env is rebuilt).
- **Failure entries** (exec error / timeout / bad JSON): cached with a 1h TTL so a
  slow or non-plugin `dr-*` binary (the exact pyenv-shim case we hit) isn't re-exec'd
  on every invocation. Same failure outcome as today, but without paying the 500ms.
- Corrupt or unreadable cache file: ignore silently, rediscover, rewrite.
- Prune entries whose paths no longer exist when saving.

## Wiring

1. `DiscoverPluginsWithContext` loads the cache once before the parallel phase
   (managed plugins need no caching, their manifests are file reads with no exec).
2. `getManifestsParallel`: for each executable, stat it; on cache hit return the
   cached result (manifest or failed-skip) without exec. On miss, exec as today;
   record the result (success or failure) for the save pass.
3. After `wg.Wait()`, persist new entries in one write. No concurrent file access.
4. Managed-plugin and local-plugin paths untouched.

## Tests (`internal/plugin/manifest_cache_test.go`)

- Round-trip: save/load preserves entries; corrupt file ignored.
- Hit: unchanged mtime+size → no exec (use an injectable `execManifest` func seam,
  same pattern as existing test doubles in this package).
- Invalidation: mtime or size change → re-exec.
- Failure caching: failed fetch skipped within TTL, retried after.
- Pruning of vanished paths.
- Existing discovery tests keep passing (`getManifest` gains a default exec seam
  so prod behavior is unchanged).

## Validation

- `task test` (internal/plugin at minimum) and `task lint`.
- Manual timing check with a deliberately slow fake `dr-*` on a temp PATH: first
  run pays the exec, second run is ~10–30ms.

## Out of scope

- Moving discovery out of package `init()` / making `--plugin-discovery-timeout`
  actually effective. That fix was implemented separately before this deferred
  cache spec.
- Any change to managed-plugin behavior.

**Expected result:** warm `dr self version --short` drops from ~620ms (with the
pyenv shim present) to ~80–120ms; cold runs unchanged from today.
