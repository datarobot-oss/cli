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

package doctor

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// healthyProject writes a fully healthy local state: valid config with both
// pointers, valid manifest agreeing with it, no rollback tree, no lock file.
func healthyProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	require.NoError(t, wapi.SaveManifest(dir, validManifest(testVersionID)))

	return dir
}

func TestLocalChecks_FixedOrder(t *testing.T) {
	ids := make([]string, 0, 6)

	for _, c := range LocalChecks(t.TempDir()) {
		ids = append(ids, c.ID())
	}

	assert.Equal(t, []string{
		CheckIDPresence,
		CheckIDConfig,
		CheckIDManifest,
		CheckIDDivergence,
		CheckIDRollback,
		CheckIDLock,
	}, ids)
}

func TestLocalChecks_Healthy_AllOK(t *testing.T) {
	results := runLocalChecks(t, healthyProject(t))

	want := []core.Status{
		core.StatusOK,
		core.StatusOK,
		core.StatusOK,
		core.StatusOK,
		core.StatusOK,
		core.StatusOK,
	}

	for i, res := range results {
		assert.Equal(t, want[i], res.Status, "check %s: summary %q", res.CheckID, res.Summary)
	}
}

func TestLocalChecks_Cascade_PresenceFailSkipsEverything(t *testing.T) {
	// Fresh dir: nothing linked, so every other check must SKIP.
	results := runLocalChecks(t, t.TempDir())

	require.Len(t, results, 6)

	assert.Equal(t, core.StatusFAIL, results[0].Status)

	for _, res := range results[1:] {
		assert.Equal(t, core.StatusSKIP, res.Status, "check %s should SKIP", res.CheckID)

		assert.Contains(t, res.Summary, "no linked state")
	}
}

func TestLocalChecks_Cascade_ConfigFailSkipsDivergenceOnly(t *testing.T) {
	dir := t.TempDir()

	// State dir present (presence OK), no config, valid manifest:
	// divergence SKIPs but manifest/rollback/lock still run.
	initStateDir(t, dir)

	require.NoError(t, wapi.SaveManifest(dir, validManifest("")))

	results := runLocalChecks(t, dir)

	require.Len(t, results, 6)

	want := []core.Status{
		core.StatusOK,   // presence
		core.StatusFAIL, // config missing
		core.StatusOK,   // manifest still runs
		core.StatusSKIP, // divergence depends on config
		core.StatusOK,   // rollback still runs
		core.StatusOK,   // lock still runs
	}

	for i, res := range results {
		assert.Equal(t, want[i], res.Status, "check %s: summary %q", res.CheckID, res.Summary)
	}
}

func TestLocalChecks_Cascade_ManifestFailSkipsDivergenceOnly(t *testing.T) {
	dir := t.TempDir()

	// Valid config, corrupt manifest: config stays OK, divergence SKIPs,
	// rollback/lock still run.
	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	writeStateFile(t, dir, "manifest.json", `{"version":1,`)

	results := runLocalChecks(t, dir)

	require.Len(t, results, 6)

	want := []core.Status{
		core.StatusOK,   // presence
		core.StatusOK,   // config
		core.StatusFAIL, // manifest corrupt
		core.StatusSKIP, // divergence depends on manifest
		core.StatusOK,   // rollback still runs
		core.StatusOK,   // lock still runs
	}

	for i, res := range results {
		assert.Equal(t, want[i], res.Status, "check %s: summary %q", res.CheckID, res.Summary)
	}
}

func TestLocalChecks_ReadOnly_ZeroWritesAndNoNewFiles(t *testing.T) {
	t.Run("state byte-identical after a full run", func(t *testing.T) {
		dir := healthyProject(t)

		// A stale rollback tree must survive a read-only diagnosis untouched.
		backedUp := filepath.Join(wapi.Dir(dir), wapi.RollbackDirName, "app", "main.go")

		require.NoError(t, os.MkdirAll(filepath.Dir(backedUp), 0o755))

		require.NoError(t, os.WriteFile(backedUp, []byte("package main\n"), 0o600))

		before := stateFileHashes(t, dir)

		runLocalChecks(t, dir)

		assert.Equal(t, before, stateFileHashes(t, dir))
	})

	t.Run("sync.lock is not created", func(t *testing.T) {
		dir := healthyProject(t)

		runLocalChecks(t, dir)

		_, err := os.Stat(filepath.Join(wapi.Dir(dir), sync.LockFileName))

		assert.ErrorIs(t, err, fs.ErrNotExist, "read-only run must not create sync.lock")
	})
}
