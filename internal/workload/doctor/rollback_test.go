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
	"context"
	"os"
	"path/filepath"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rollbackDir(t *testing.T, projectDir string) string {
	t.Helper()

	return filepath.Join(wapi.Dir(projectDir), wapi.RollbackDirName)
}

func TestRollbackCheck_OK_NoRollbackDir(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	res := (&rollbackCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	assert.Empty(t, res.Remedy)
}

func TestRollbackCheck_FAIL_StaleDirWithFiles(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	backedUp := filepath.Join(rollbackDir(t, dir), "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(backedUp), 0o755))

	require.NoError(t, os.WriteFile(backedUp, []byte("package main\n"), 0o600))

	res := (&rollbackCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	assert.Equal(t, RemedyRollback, res.Remedy)

	assert.True(t, res.Fixable)
}

func TestRollbackCheck_FAIL_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	// An empty .rollback/ still means an interrupted rollback: the dir
	// itself is the evidence, not its contents.
	require.NoError(t, os.MkdirAll(rollbackDir(t, dir), 0o755))

	res := (&rollbackCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)
}

func TestRollbackCheck_FAIL_LegacyPath(t *testing.T) {
	dir := t.TempDir()

	// Legacy-only project: state lives in .wapi, so the stale rollback tree
	// hides under .wapi/.rollback.
	legacyRollback := filepath.Join(dir, wapi.LegacyDirName, wapi.RollbackDirName)

	require.NoError(t, os.MkdirAll(legacyRollback, 0o755))

	writeStateFile(t, dir, "config.json", validConfigJSON())

	// wapi.Dir now resolves to the legacy location; sanity-check that the
	// state file landed there before asserting the check sweeps it.
	_, err := os.Stat(filepath.Join(dir, wapi.LegacyDirName, "config.json"))

	require.NoError(t, err)

	res := (&rollbackCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)
}

func TestRollbackCheck_SKIP_NotLinked(t *testing.T) {
	res := (&rollbackCheck{projectDir: t.TempDir()}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)

	assert.Contains(t, res.Summary, "no linked state")
}
