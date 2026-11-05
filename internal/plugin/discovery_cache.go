// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

// DiscoveryCacheKey is the global flag/env/config key for the plugin
// discovery cache TTL. Like plugin-discovery-timeout, config-file values are
// read too late to affect startup discovery; the leading flag and the env
// var are the effective controls.
const DiscoveryCacheKey = "plugin-discovery-cache-ttl"

// DiscoveryCacheEnvVar is read directly (bypassing viper) because startup
// discovery runs before config initialization, mirroring the discovery
// timeout handling.
const DiscoveryCacheEnvVar = "DATAROBOT_CLI_PLUGIN_DISCOVERY_CACHE_TTL"

// DefaultDiscoveryCacheTTL is the cache TTL used when no flag/env/config
// override applies. Entries also invalidate immediately when the executable's
// fingerprint changes, so the TTL only bounds staleness in edge cases.
const DefaultDiscoveryCacheTTL = 24 * time.Hour

const (
	// cacheFileVersion guards against loading cache files written by
	// incompatible schema versions.
	cacheFileVersion = 1

	// maxCacheFileSize caps the cache file read at load time; a larger file
	// is treated as corrupt (or tampered) and discarded.
	maxCacheFileSize = 1 << 20 // 1 MiB

	// maxCacheEntries caps the number of cache entries accepted at load time.
	maxCacheEntries = 10000

	// cacheDirName is the subdir under the XDG state dir, matching the
	// dotenv-backups precedent (~/.local/state/dr/).
	cacheDirName = "dr"

	// cacheFileBaseName is the cache file name within cacheDirName.
	cacheFileBaseName = "plugin-discovery-cache.json"
)

// cacheFingerprint captures the on-disk state of an executable at probe time.
// ResolvedPath records the symlink target so repointed links (e.g. Homebrew
// upgrades) invalidate the entry even when mtime/size are unchanged.
type cacheFingerprint struct {
	Mtime        time.Time `json:"mtime"`
	Size         int64     `json:"size"`
	ResolvedPath string    `json:"resolved_path"`
}

// cacheEntry is one cached manifest probe result. Exactly one of Manifest or
// ProbeError is set: a ProbeError entry is a negative cache hit, so binaries
// that are not plugins (e.g. pyenv shims) stop costing a probe every run.
type cacheEntry struct {
	Manifest    *PluginManifest  `json:"manifest,omitempty"`
	ProbeError  string           `json:"probe_error,omitempty"`
	Fingerprint cacheFingerprint `json:"fingerprint"`
	FetchedAt   time.Time        `json:"fetched_at"`
}

// discoveryCacheFile is the on-disk cache format.
type discoveryCacheFile struct {
	Version int                    `json:"version"`
	Entries map[string]*cacheEntry `json:"entries"`
}

// DiscoveryCache caches manifest probe results per executable path. A nil
// *DiscoveryCache is a fully functional disabled cache: every lookup misses
// and every record is a no-op.
//
// Concurrency: PATH directories are probed concurrently, and each directory's
// merge records results while other directories still look up, so entries and
// dirty are guarded by an RWMutex (lookups take the read side). Save happens
// on the calling goroutine after all probe work completes.
type DiscoveryCache struct {
	path string
	ttl  time.Duration

	mu      sync.RWMutex
	entries map[string]*cacheEntry
	dirty   bool
}

// NewDiscoveryCache returns a cache backed by path with the given TTL.
// The cache file is not read until load is called.
func NewDiscoveryCache(path string, ttl time.Duration) *DiscoveryCache {
	return &DiscoveryCache{
		path:    path,
		ttl:     ttl,
		entries: make(map[string]*cacheEntry),
	}
}

// DiscoveryCachePath returns the cache file path under the XDG state dir.
func DiscoveryCachePath() (string, error) {
	stateDir, err := config.GetStateDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve plugin discovery cache location: %w", err)
	}

	return filepath.Join(stateDir, cacheDirName, cacheFileBaseName), nil
}

// overrideDiscoveryCacheTTL caches the TTL resolved at startup from the
// leading global flag. It is set once by RegisterPluginCommands before any
// discovery runs, so no synchronization is needed; it is deliberately not
// goroutine-safe.
var (
	overrideDiscoveryCacheTTL    time.Duration
	overrideDiscoveryCacheTTLSet bool
)

// SetDiscoveryCacheTTL records the startup-resolved cache TTL (flag first,
// then env/config/default). Must be called before DiscoverPluginsWithContext;
// production callers do this in RegisterPluginCommands.
func SetDiscoveryCacheTTL(ttl time.Duration) {
	overrideDiscoveryCacheTTL = ttl
	overrideDiscoveryCacheTTLSet = true
}

// resetDiscoveryCacheTTL clears the startup override. Test-only.
func resetDiscoveryCacheTTL() {
	overrideDiscoveryCacheTTL = 0
	overrideDiscoveryCacheTTLSet = false
}

// DiscoveryCacheTTL resolves the discovery cache TTL. The startup override
// (flag) wins; otherwise the env var is checked explicitly because command
// registration runs before viper's env binding is initialized; viper remains
// the fallback for lazy GetPlugins calls that happen after config
// initialization.
func DiscoveryCacheTTL() time.Duration {
	if overrideDiscoveryCacheTTLSet {
		return overrideDiscoveryCacheTTL
	}

	if raw := os.Getenv(DiscoveryCacheEnvVar); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			log.Debug("Invalid plugin discovery cache TTL environment value", "value", raw, "error", err)

			return DefaultDiscoveryCacheTTL
		}

		return ttl
	}

	if viperx.IsSet(DiscoveryCacheKey) {
		return viperx.GetDuration(DiscoveryCacheKey)
	}

	return DefaultDiscoveryCacheTTL
}

// sharedDiscoveryCache returns the cache for one discovery run, or nil when
// the cache is disabled (TTL <= 0) or its location cannot be resolved.
func sharedDiscoveryCache() *DiscoveryCache {
	ttl := DiscoveryCacheTTL()
	if ttl <= 0 {
		log.Debug("Plugin discovery cache disabled", "ttl", ttl)

		return nil
	}

	path, err := DiscoveryCachePath()
	if err != nil {
		log.Debug("Plugin discovery cache unavailable", "error", err)

		return nil
	}

	cache := NewDiscoveryCache(path, ttl)

	cache.load()

	return cache
}

// load reads the cache file. Any problem (missing, unreadable, oversized,
// corrupt, wrong version, invalid entries) results in an empty or partial
// cache: a cache must never break discovery, only accelerate it.
func (c *DiscoveryCache) load() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debug("Plugin discovery cache unreadable; starting fresh", "path", c.path, "error", err)
		}

		return
	}

	if len(data) > maxCacheFileSize {
		log.Debug("Plugin discovery cache oversized; discarding", "path", c.path, "bytes", len(data))

		return
	}

	var file discoveryCacheFile

	if err := json.Unmarshal(data, &file); err != nil {
		log.Debug("Plugin discovery cache corrupt; discarding", "path", c.path, "error", err)

		return
	}

	if file.Version != cacheFileVersion {
		log.Debug("Plugin discovery cache version mismatch; discarding", "path", c.path, "version", file.Version)

		return
	}

	if len(file.Entries) > maxCacheEntries {
		log.Debug("Plugin discovery cache has too many entries; discarding", "path", c.path, "entries", len(file.Entries))

		return
	}

	for execPath, entry := range file.Entries {
		if !entry.valid() {
			log.Debug("Dropping invalid plugin discovery cache entry", "path", execPath)

			continue
		}

		c.entries[execPath] = entry
	}

	log.Debug("Plugin discovery cache loaded", "path", c.path, "entries", len(c.entries))
}

// valid reports whether an entry carries a usable result.
func (e *cacheEntry) valid() bool {
	if e == nil || e.FetchedAt.IsZero() || e.Fingerprint.Mtime.IsZero() {
		return false
	}

	if e.Manifest != nil && e.Manifest.Name == "" {
		return false
	}

	return e.Manifest != nil || e.ProbeError != ""
}

// fingerprintMatches re-validates an entry against the executable's current
// on-disk state, logging the invalidation reason on mismatch.
func (e *cacheEntry) fingerprintMatches(executable string) bool {
	info, err := os.Stat(executable)
	if err != nil {
		log.Debug("Plugin discovery cache entry invalidated: executable missing", "path", executable)

		return false
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		log.Debug("Plugin discovery cache entry invalidated: symlink resolution failed", "path", executable, "error", err)

		return false
	}

	if !info.ModTime().Equal(e.Fingerprint.Mtime) || info.Size() != e.Fingerprint.Size {
		log.Debug("Plugin discovery cache entry invalidated: executable changed", "path", executable)

		return false
	}

	if resolved != e.Fingerprint.ResolvedPath {
		log.Debug("Plugin discovery cache entry invalidated: symlink target changed", "path", executable, "resolved", resolved)

		return false
	}

	return true
}

// lookup returns the cached entry for executable if it is fresh and the
// executable's fingerprint still matches, logging the invalidation reason on
// a miss. It never mutates the cache.
func (c *DiscoveryCache) lookup(executable string) (*cacheEntry, bool) {
	if c == nil {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[executable]
	if !ok {
		return nil, false
	}

	if time.Since(entry.FetchedAt) >= c.ttl {
		log.Debug("Plugin discovery cache entry expired", "path", executable, "fetched_at", entry.FetchedAt)

		return nil, false
	}

	if !entry.fingerprintMatches(executable) {
		return nil, false
	}

	return entry, true
}

// record stores a probe result for executable. Called from per-directory
// merge loops that may run concurrently, so it takes the write lock.
// Entries for executables that cannot be stat'd are silently skipped.
func (c *DiscoveryCache) record(executable string, manifest *PluginManifest, probeErr error) {
	if c == nil || executable == "" {
		return
	}

	info, err := os.Stat(executable)
	if err != nil {
		return
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return
	}

	entry := &cacheEntry{
		Fingerprint: cacheFingerprint{
			Mtime:        info.ModTime(),
			Size:         info.Size(),
			ResolvedPath: resolved,
		},
		FetchedAt: time.Now().UTC(),
	}

	switch {
	case probeErr != nil:
		entry.ProbeError = probeErr.Error()
	case manifest != nil:
		entry.Manifest = manifest
	default:
		// Cancelled probe: nothing to record.
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.entries[executable]; ok && existing.sameResult(entry) {
		return
	}

	c.entries[executable] = entry
	c.dirty = true
}

// sameResult reports whether two entries carry an identical probe outcome
// (fingerprint and fetched_at are not compared: re-probing an unchanged
// binary with the same outcome should not dirty the cache).
func (e *cacheEntry) sameResult(other *cacheEntry) bool {
	if e == nil || other == nil {
		return e == other
	}

	if e.ProbeError != other.ProbeError {
		return false
	}

	return reflect.DeepEqual(e.Manifest, other.Manifest)
}

// saveIfDirty persists the cache when any result changed. Entries whose
// executable no longer exists are pruned first. All errors are best-effort:
// a failed save costs a re-probe on the next run, nothing more.
// Save runs on the discovery goroutine after all probe work completes, but
// it takes the write lock anyway so it is safe against any future caller.
func (c *DiscoveryCache) saveIfDirty() {
	if c == nil || !c.dirty {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pruneMissing()

	if len(c.entries) > maxCacheEntries {
		log.Debug("Plugin discovery cache too large to save; skipping", "path", c.path, "entries", len(c.entries))

		return
	}

	data, err := json.MarshalIndent(discoveryCacheFile{
		Version: cacheFileVersion,
		Entries: c.entries,
	}, "", "  ")
	if err != nil {
		log.Debug("Plugin discovery cache marshal failed", "error", err)

		return
	}

	if err := c.writeAtomic(data); err != nil {
		log.Debug("Plugin discovery cache save failed", "path", c.path, "error", err)

		return
	}

	log.Debug("Plugin discovery cache saved", "path", c.path, "entries", len(c.entries), "bytes", len(data))
}

// writeAtomic writes data to the cache path via a temp file in the same
// directory plus rename, so concurrent readers never observe a partially
// written cache.
func (c *DiscoveryCache) writeAtomic(data []byte) error {
	dir := filepath.Dir(c.path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, cacheFileBaseName+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	// After a successful rename, both cleanup steps are no-ops: the handle
	// is already closed and the temp path no longer exists.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache: %w", err)
	}

	if err := os.Rename(tmp.Name(), c.path); err != nil {
		return fmt.Errorf("rename cache: %w", err)
	}

	return nil
}

// pruneMissing drops entries whose executable no longer exists so deleted
// binaries do not accumulate stale entries forever.
func (c *DiscoveryCache) pruneMissing() {
	for execPath := range c.entries {
		if _, err := os.Stat(execPath); err != nil {
			log.Debug("Pruning plugin discovery cache entry for missing executable", "path", execPath)

			delete(c.entries, execPath)
			c.dirty = true
		}
	}
}

// CacheStats is a read-only summary of the on-disk discovery cache, for
// reporting commands. Zero values mean the file is missing or unreadable.
type CacheStats struct {
	Exists        bool
	SizeBytes     int64
	OkEntries     int
	FailedEntries int
	OldestFetch   time.Time
	NewestFetch   time.Time
}

// InspectCache reads the cache file at path and summarizes its contents.
// A corrupt file reports Exists/SizeBytes but no entry stats, mirroring how
// discovery treats it (discard and rebuild).
func InspectCache(path string) CacheStats {
	stats := CacheStats{}

	data, err := os.ReadFile(path)
	if err != nil {
		return stats
	}

	stats.Exists = true
	stats.SizeBytes = int64(len(data))

	var file discoveryCacheFile

	if err := json.Unmarshal(data, &file); err != nil {
		return stats
	}

	for _, entry := range file.Entries {
		if !entry.valid() {
			continue
		}

		if entry.ProbeError != "" {
			stats.FailedEntries++
		} else {
			stats.OkEntries++
		}

		if stats.OldestFetch.IsZero() || entry.FetchedAt.Before(stats.OldestFetch) {
			stats.OldestFetch = entry.FetchedAt
		}

		if entry.FetchedAt.After(stats.NewestFetch) {
			stats.NewestFetch = entry.FetchedAt
		}
	}

	return stats
}
