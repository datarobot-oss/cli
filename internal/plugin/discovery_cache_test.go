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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// CacheCoreTestSuite tests the on-disk discovery cache in isolation.
type CacheCoreTestSuite struct {
	suite.Suite

	tempDir      string
	cacheDir     string // directory the cache file lives in
	cacheDirBase string
}

func TestCacheCoreTestSuite(t *testing.T) {
	suite.Run(t, new(CacheCoreTestSuite))
}

func (s *CacheCoreTestSuite) SetupTest() {
	var err error

	s.tempDir, err = os.MkdirTemp("", "plugin-cache-test")
	s.Require().NoError(err)

	s.cacheDir = filepath.Join(s.tempDir, "state", "dr")
	s.cacheDirBase = filepath.Join(s.cacheDir, cacheFileBaseName)

	viperx.Reset()
}

func (s *CacheCoreTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}

	resetDiscoveryCacheTTL()

	viperx.Reset()
}

// newCache returns a cache backed by the suite's temp cache path.
func (s *CacheCoreTestSuite) newCache(ttl time.Duration) *DiscoveryCache {
	return NewDiscoveryCache(s.cacheDirBase, ttl)
}

func (s *CacheCoreTestSuite) TestRoundTrip() {
	cache := s.newCache(24 * time.Hour)

	exe := filepath.Join(s.tempDir, "dr-foo")
	createScript(s.T(), exe, "#!/bin/sh\nexit 0\n")

	manifest := &PluginManifest{}
	manifest.Name = "foo"
	manifest.Version = "1.0.0"

	cache.record(exe, manifest, nil)
	cache.saveIfDirty()

	// File exists with owner-only permissions.
	info, err := os.Stat(s.cacheDirBase)
	s.Require().NoError(err)

	if runtime.GOOS != "windows" {
		s.Equal(os.FileMode(0o600), info.Mode().Perm())
	}

	// A fresh cache instance sees the recorded manifest.
	reloaded := s.newCache(24 * time.Hour)

	reloaded.load()

	entry, ok := reloaded.lookup(exe)
	s.Require().True(ok, "cache hit expected after round-trip")
	s.Require().NotNil(entry.Manifest)
	s.Equal("foo", entry.Manifest.Name)
	s.Equal("1.0.0", entry.Manifest.Version)
	s.False(reloaded.dirty, "lookup must not dirty the cache")
}

func (s *CacheCoreTestSuite) TestNegativeEntryRoundTrip() {
	cache := s.newCache(24 * time.Hour)

	exe := filepath.Join(s.tempDir, "dr-notaplugin")
	createScript(s.T(), exe, "#!/bin/sh\nexit 1\n")

	probeErr := errors.New("exit status 1")

	cache.record(exe, nil, probeErr)
	cache.saveIfDirty()

	reloaded := s.newCache(24 * time.Hour)

	reloaded.load()

	entry, ok := reloaded.lookup(exe)
	s.Require().True(ok, "cached failure must be a hit")
	s.Empty(entry.Manifest)
	s.NotEmpty(entry.ProbeError)
}

func (s *CacheCoreTestSuite) TestTTLExpiry() {
	exe := filepath.Join(s.tempDir, "dr-foo")
	createScript(s.T(), exe, "#!/bin/sh\nexit 0\n")

	manifest := &PluginManifest{}
	manifest.Name = "foo"

	cache := s.newCache(time.Hour)

	cache.record(exe, manifest, nil)
	cache.saveIfDirty()

	// The same file viewed through an already-expired TTL misses.
	expired := s.newCache(-time.Minute)

	expired.load()

	_, ok := expired.lookup(exe)
	s.False(ok, "expired entry must miss")
}

func (s *CacheCoreTestSuite) TestInvalidatedOnMtimeChange() {
	cache := s.newCache(24 * time.Hour)

	exe := filepath.Join(s.tempDir, "dr-foo")
	createScript(s.T(), exe, "#!/bin/sh\nexit 0\n")

	manifest := &PluginManifest{}
	manifest.Name = "foo"

	cache.record(exe, manifest, nil)

	// Bump mtime into the future without changing size.
	future := time.Now().Add(time.Hour)
	s.Require().NoError(os.Chtimes(exe, future, future))

	_, ok := cache.lookup(exe)
	s.False(ok, "mtime change must invalidate the entry")
}

func (s *CacheCoreTestSuite) TestInvalidatedOnSizeChange() {
	cache := s.newCache(24 * time.Hour)

	exe := filepath.Join(s.tempDir, "dr-foo")
	createScript(s.T(), exe, "#!/bin/sh\nexit 0\n")

	manifest := &PluginManifest{}
	manifest.Name = "foo"

	cache.record(exe, manifest, nil)

	// Same mtime, different size.
	info, err := os.Stat(exe)
	s.Require().NoError(err)

	s.Require().NoError(os.Chtimes(exe, info.ModTime(), info.ModTime()))
	s.Require().NoError(os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n#extra\n"), 0o755))
	s.Require().NoError(os.Chtimes(exe, info.ModTime(), info.ModTime()))

	_, ok := cache.lookup(exe)
	s.False(ok, "size change must invalidate the entry")
}

func (s *CacheCoreTestSuite) TestInvalidatedOnSymlinkRepoint() {
	if runtime.GOOS == "windows" {
		s.T().Skip("symlink repoint test requires POSIX symlinks")
	}

	target1 := filepath.Join(s.tempDir, "real-dr-foo-1")
	target2 := filepath.Join(s.tempDir, "real-dr-foo-2")

	createScript(s.T(), target1, "#!/bin/sh\nexit 0\n")
	createScript(s.T(), target2, "#!/bin/sh\nexit 0\n")

	link := filepath.Join(s.tempDir, "dr-foo")
	s.Require().NoError(os.Symlink(target1, link))

	cache := s.newCache(24 * time.Hour)

	manifest := &PluginManifest{}
	manifest.Name = "foo"

	cache.record(link, manifest, nil)

	_, ok := cache.lookup(link)
	s.Require().True(ok, "entry must hit while the symlink is unchanged")

	// Repoint the symlink to a different target with identical content.
	s.Require().NoError(os.Remove(link))
	s.Require().NoError(os.Symlink(target2, link))

	_, ok = cache.lookup(link)
	s.False(ok, "repointed symlink must invalidate the entry")
}

func (s *CacheCoreTestSuite) TestInvalidatedOnMissingExecutable() {
	cache := s.newCache(24 * time.Hour)

	exe := filepath.Join(s.tempDir, "dr-foo")
	createScript(s.T(), exe, "#!/bin/sh\nexit 0\n")

	manifest := &PluginManifest{}
	manifest.Name = "foo"

	cache.record(exe, manifest, nil)

	s.Require().NoError(os.Remove(exe))

	_, ok := cache.lookup(exe)
	s.False(ok, "missing executable must invalidate the entry")
}

func (s *CacheCoreTestSuite) TestCorruptFileDiscarded() {
	s.Require().NoError(os.MkdirAll(s.cacheDir, 0o755))
	s.Require().NoError(os.WriteFile(s.cacheDirBase, []byte("this is not json"), 0o600))

	cache := s.newCache(24 * time.Hour)

	cache.load()

	s.Empty(cache.entries, "corrupt cache must load as empty")
}

func (s *CacheCoreTestSuite) TestVersionMismatchDiscarded() {
	s.Require().NoError(os.MkdirAll(s.cacheDir, 0o755))

	data, err := json.Marshal(discoveryCacheFile{Version: cacheFileVersion + 1, Entries: map[string]*cacheEntry{}})
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(s.cacheDirBase, data, 0o600))

	cache := s.newCache(24 * time.Hour)

	cache.load()

	s.Empty(cache.entries, "future-version cache must load as empty")
}

func (s *CacheCoreTestSuite) TestOversizedFileDiscarded() {
	s.Require().NoError(os.MkdirAll(s.cacheDir, 0o755))
	s.Require().NoError(os.WriteFile(s.cacheDirBase, make([]byte, maxCacheFileSize+1), 0o600))

	cache := s.newCache(24 * time.Hour)

	cache.load()

	s.Empty(cache.entries, "oversized cache must load as empty")
}

func (s *CacheCoreTestSuite) TestInvalidEntriesDroppedOnLoad() {
	valid := &cacheEntry{
		Manifest: &PluginManifest{},
		Fingerprint: cacheFingerprint{
			Mtime: time.Now(),
		},
		FetchedAt: time.Now(),
	}

	valid.Manifest.Name = "ok"

	noName := &cacheEntry{
		Manifest:    &PluginManifest{},
		Fingerprint: cacheFingerprint{Mtime: time.Now()},
		FetchedAt:   time.Now(),
	}

	empty := &cacheEntry{
		Fingerprint: cacheFingerprint{Mtime: time.Now()},
		FetchedAt:   time.Now(),
	}

	noTimestamp := &cacheEntry{
		Manifest:    &PluginManifest{},
		Fingerprint: cacheFingerprint{Mtime: time.Now()},
	}

	data, err := json.Marshal(discoveryCacheFile{
		Version: cacheFileVersion,
		Entries: map[string]*cacheEntry{
			"/x/ok":           valid,
			"/x/no-name":      noName,
			"/x/empty":        empty,
			"/x/no-timestamp": noTimestamp,
		},
	})
	s.Require().NoError(err)

	s.Require().NoError(os.MkdirAll(s.cacheDir, 0o755))
	s.Require().NoError(os.WriteFile(s.cacheDirBase, data, 0o600))

	cache := s.newCache(24 * time.Hour)

	cache.load()

	s.Len(cache.entries, 1, "only the valid entry survives load")
	_, ok := cache.entries["/x/ok"]
	s.True(ok)
}

func (s *CacheCoreTestSuite) TestSaveIfDirtyOnlyWhenDirty() {
	cache := s.newCache(24 * time.Hour)

	cache.saveIfDirty()

	_, err := os.Stat(s.cacheDirBase)
	s.True(os.IsNotExist(err), "clean cache must not write a file")
}

func (s *CacheCoreTestSuite) TestSavePrunesMissingExecutables() {
	kept := filepath.Join(s.tempDir, "dr-keep")
	gone := filepath.Join(s.tempDir, "dr-gone")

	createScript(s.T(), kept, "#!/bin/sh\nexit 0\n")
	createScript(s.T(), gone, "#!/bin/sh\nexit 0\n")

	cache := s.newCache(24 * time.Hour)

	manifest := &PluginManifest{}
	manifest.Name = "keep"

	cache.record(kept, manifest, nil)
	cache.record(gone, manifest, nil)
	cache.saveIfDirty()

	s.Require().NoError(os.Remove(gone))

	// A later unrelated change makes the cache dirty again; the save must
	// drop the entry for the deleted binary.
	other := filepath.Join(s.tempDir, "dr-other")
	createScript(s.T(), other, "#!/bin/sh\nexit 0\n")

	cache.record(other, manifest, nil)
	cache.saveIfDirty()

	reloaded := s.newCache(24 * time.Hour)

	reloaded.load()

	_, ok := reloaded.entries[gone]
	s.False(ok, "entry for deleted executable must be pruned")
	_, ok = reloaded.entries[kept]
	s.True(ok)
	_, ok = reloaded.entries[other]
	s.True(ok)
}

func (s *CacheCoreTestSuite) TestNilCacheIsNoop() {
	var cache *DiscoveryCache

	s.Nil(cache.lookup("/anything"))
	cache.record("/anything", nil, nil)
	cache.saveIfDirty()
}

func (s *CacheCoreTestSuite) TestCachePathFollowsXDGStateHome() {
	testutil.SetXDGEnv(s.T(), "XDG_STATE_HOME", s.tempDir)

	path, err := DiscoveryCachePath()
	s.Require().NoError(err)
	s.Equal(filepath.Join(s.tempDir, "dr", cacheFileBaseName), path)
}

func (s *CacheCoreTestSuite) TestTTLResolution() {
	// Default.
	s.Equal(DefaultDiscoveryCacheTTL, DiscoveryCacheTTL())

	// Env var (read directly, no viper involvement).
	testutil.SetXDGEnv(s.T(), "XDG_STATE_HOME", s.tempDir)

	s.T().Setenv(DiscoveryCacheEnvVar, "25ms")
	s.Equal(25*time.Millisecond, DiscoveryCacheTTL())

	s.T().Setenv(DiscoveryCacheEnvVar, "not-a-duration")
	s.Equal(DefaultDiscoveryCacheTTL, DiscoveryCacheTTL())

	s.T().Setenv(DiscoveryCacheEnvVar, "")
	s.Equal(DefaultDiscoveryCacheTTL, DiscoveryCacheTTL())

	// Startup override (flag), including 0s = disabled.
	SetDiscoveryCacheTTL(0)
	s.Equal(time.Duration(0), DiscoveryCacheTTL())

	SetDiscoveryCacheTTL(5 * time.Minute)
	s.Equal(5*time.Minute, DiscoveryCacheTTL())
}

func (s *CacheCoreTestSuite) TestSharedCacheDisabledOnZeroTTL() {
	SetDiscoveryCacheTTL(0)

	s.Nil(sharedDiscoveryCache())
}

// writeCountingPluginScript creates a plugin script that appends a byte to
// counterFile on every invocation, so tests can assert exactly how many
// times the binary was executed.
func writeCountingPluginScript(t *testing.T, dir, name, manifestJSON, counterFile string) string {
	t.Helper()

	var scriptPath, scriptContent string

	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(dir, name+".ps1")
		scriptContent = fmt.Sprintf("Add-Content -Path '%s' -Value 'x'\n"+
			"if ($args[0] -eq '--dr-plugin-manifest') {\n"+
			"  Write-Output '%s'\n"+
			"}\n", counterFile, manifestJSON)
	} else {
		scriptPath = filepath.Join(dir, name)
		scriptContent = fmt.Sprintf("#!/bin/sh\nprintf x >> '%s'\n"+
			"if [ \"$1\" = \"--dr-plugin-manifest\" ]; then\n"+
			"  echo '%s'\n"+
			"fi\n", counterFile, manifestJSON)
	}

	createScript(t, scriptPath, scriptContent)

	return scriptPath
}

// countProbes reads the probe counter file (0 when absent).
func countProbes(t *testing.T, counterFile string) int {
	t.Helper()

	data, err := os.ReadFile(counterFile)
	if err != nil {
		return 0
	}

	return len(data)
}

// CacheIntegrationTestSuite exercises discovery end-to-end with the cache.
type CacheIntegrationTestSuite struct {
	suite.Suite

	tempDir     string
	pluginDir   string
	counterFile string
}

func TestCacheIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CacheIntegrationTestSuite))
}

func (s *CacheIntegrationTestSuite) SetupTest() {
	var err error

	s.tempDir, err = os.MkdirTemp("", "plugin-cache-integration")
	s.Require().NoError(err)

	s.pluginDir = filepath.Join(s.tempDir, "bin")
	s.counterFile = filepath.Join(s.tempDir, "probes.txt")

	s.Require().NoError(os.MkdirAll(s.pluginDir, 0o755))

	viperx.Reset()
	viperx.Set("plugin.manifest_timeout_ms", 5000)

	// Isolate home/config (managed plugin dirs) and route the cache into
	// the temp state dir.
	testutil.SetTestHomeDir(s.T(), s.tempDir)
	testutil.SetXDGEnv(s.T(), "XDG_STATE_HOME", s.tempDir)

	SetDiscoveryCacheTTL(24 * time.Hour)

	setDiscoveryPath(s.T(), s.pluginDir)
}

func (s *CacheIntegrationTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}

	resetDiscoveryCacheTTL()

	viperx.Reset()
}

// discover runs one discovery pass and returns the discovered plugin names.
func (s *CacheIntegrationTestSuite) discover() []string {
	plugins, _ := DiscoverPluginsWithContext(context.Background())

	names := make([]string, 0, len(plugins))

	for _, p := range plugins {
		names = append(names, p.Manifest.Name)
	}

	return names
}

func (s *CacheIntegrationTestSuite) TestSecondRunDoesNotReprobe() {
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-cached", `{"name":"cached","version":"1.0.0"}`, s.counterFile)

	s.Equal([]string{"cached"}, s.discover())
	s.Equal(1, countProbes(s.T(), s.counterFile), "first run probes the binary")

	s.Equal([]string{"cached"}, s.discover())
	s.Equal(1, countProbes(s.T(), s.counterFile), "second run must use the cache, not re-execute the binary")
}

func (s *CacheIntegrationTestSuite) TestFailedProbeIsCached() {
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-fail", `{"name":"fail"}`, s.counterFile)
	writeExitScript(s.T(), s.pluginDir, "dr-shim", 1)

	s.discover()
	probesAfterFirst := countProbes(s.T(), s.counterFile)

	s.discover()

	s.Equal(probesAfterFirst, countProbes(s.T(), s.counterFile),
		"failed probes (shims) must be cached too and not re-executed")
}

func (s *CacheIntegrationTestSuite) TestNewBinaryIsDiscoveredImmediately() {
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-first", `{"name":"first"}`, s.counterFile)

	s.Equal([]string{"first"}, s.discover())

	// A new binary appears without waiting for the TTL: the directory scan
	// still runs, only the probe is cached.
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-second", `{"name":"second"}`, s.counterFile)

	names := s.discover()

	s.Contains(names, "first")
	s.Contains(names, "second", "new plugin must be discovered without TTL wait")
}

func (s *CacheIntegrationTestSuite) TestDeletedBinaryDisappearsImmediately() {
	script := writeCountingPluginScript(s.T(), s.pluginDir, "dr-temp", `{"name":"temp"}`, s.counterFile)

	s.Equal([]string{"temp"}, s.discover())

	s.Require().NoError(os.Remove(script))

	s.Empty(s.discover(), "deleted plugin must disappear without TTL wait")
}

func (s *CacheIntegrationTestSuite) TestDisabledCacheReprobesEveryRun() {
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-nocache", `{"name":"nocache"}`, s.counterFile)

	SetDiscoveryCacheTTL(0)

	s.Equal([]string{"nocache"}, s.discover())
	s.Equal([]string{"nocache"}, s.discover())

	s.Equal(2, countProbes(s.T(), s.counterFile), "TTL 0s disables the cache: every run re-probes")

	_, err := os.Stat(filepath.Join(s.tempDir, "dr", cacheFileBaseName))
	s.True(os.IsNotExist(err), "disabled cache must not write a cache file")
}

func (s *CacheIntegrationTestSuite) TestCacheDisabledByEnvZero() {
	writeCountingPluginScript(s.T(), s.pluginDir, "dr-envzero", `{"name":"envzero"}`, s.counterFile)

	resetDiscoveryCacheTTL()
	s.T().Setenv(DiscoveryCacheEnvVar, "0s")

	s.discover()
	s.discover()

	s.Equal(2, countProbes(s.T(), s.counterFile), "env var 0s must disable the cache at startup resolution")
}

func TestInspectCache(t *testing.T) {
	tempDir := t.TempDir()

	path := filepath.Join(tempDir, cacheFileBaseName)

	// Missing file.
	stats := InspectCache(path)
	assert.False(t, stats.Exists)

	// File with one ok and one failed entry.
	manifest := &PluginManifest{}
	manifest.Name = "ok"

	now := time.Now().UTC()

	file := discoveryCacheFile{
		Version: cacheFileVersion,
		Entries: map[string]*cacheEntry{
			"/a": {
				Manifest:    manifest,
				Fingerprint: cacheFingerprint{Mtime: now},
				FetchedAt:   now,
			},
			"/b": {
				ProbeError:  "exit status 1",
				Fingerprint: cacheFingerprint{Mtime: now},
				FetchedAt:   now,
			},
		},
	}

	data, err := json.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	stats = InspectCache(path)

	assert.True(t, stats.Exists)
	assert.Equal(t, int64(len(data)), stats.SizeBytes)
	assert.Equal(t, 1, stats.OkEntries)
	assert.Equal(t, 1, stats.FailedEntries)
	assert.False(t, stats.OldestFetch.IsZero())
	assert.False(t, stats.NewestFetch.IsZero())
}
