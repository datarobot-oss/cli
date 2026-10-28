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
	"path/filepath"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestCheck_OK(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveManifest(dir, validManifest(testVersionID)))

	res := (&manifestCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	assert.Empty(t, res.Remedy)
}

func TestManifestCheck_FAIL_Missing(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	res := (&manifestCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	assert.Contains(t, res.Summary, "missing")

	assert.Equal(t, RemedyManifest, res.Remedy)

	assert.True(t, res.Fixable)

	wantPath, err := filepath.Abs(wapi.ManifestPath(dir))

	require.NoError(t, err)

	assert.Equal(t, wantPath, res.Details["path"])
}

func TestManifestCheck_FAIL_CorruptShowsAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	writeStateFile(t, dir, "manifest.json", `{"version":1,`)

	res := (&manifestCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	wantPath, err := filepath.Abs(wapi.ManifestPath(dir))

	require.NoError(t, err)

	assert.Equal(t, wantPath, res.Details["path"])

	assert.Contains(t, res.Summary, wantPath)
}

func TestManifestCheck_FAIL_InvalidSemantics(t *testing.T) {
	t.Run("wrong version", func(t *testing.T) {
		dir := t.TempDir()

		writeStateFile(t, dir, "manifest.json", `{"version":2,"syncedAt":null,"syncedVersionId":null,"files":{}}`)

		res := (&manifestCheck{projectDir: dir}).Run(context.Background())

		assert.Equal(t, core.StatusFAIL, res.Status)

		wantPath, err := filepath.Abs(wapi.ManifestPath(dir))

		require.NoError(t, err)

		assert.Equal(t, wantPath, res.Details["path"])
	})

	t.Run("syncedVersionId without syncedAt", func(t *testing.T) {
		dir := t.TempDir()

		writeStateFile(t, dir, "manifest.json",
			`{"version":1,"syncedAt":null,"syncedVersionId":"`+testVersionID+`","files":{}}`)

		res := (&manifestCheck{projectDir: dir}).Run(context.Background())

		assert.Equal(t, core.StatusFAIL, res.Status)
	})

	t.Run("syncedAt without syncedVersionId", func(t *testing.T) {
		dir := t.TempDir()

		writeStateFile(t, dir, "manifest.json",
			`{"version":1,"syncedAt":"2026-01-01T00:00:00Z","syncedVersionId":null,"files":{}}`)

		res := (&manifestCheck{projectDir: dir}).Run(context.Background())

		assert.Equal(t, core.StatusFAIL, res.Status)
	})

	t.Run("invalid FileMeta hash", func(t *testing.T) {
		dir := t.TempDir()

		writeStateFile(t, dir, "manifest.json",
			`{"version":1,"syncedAt":null,"syncedVersionId":null,"files":{"app/main.go":{"hash":"nothex","size":3}}}`)

		res := (&manifestCheck{projectDir: dir}).Run(context.Background())

		assert.Equal(t, core.StatusFAIL, res.Status)

		wantPath, err := filepath.Abs(wapi.ManifestPath(dir))

		require.NoError(t, err)

		assert.Equal(t, wantPath, res.Details["path"])
	})
}

func TestManifestCheck_SKIP_NotLinked(t *testing.T) {
	res := (&manifestCheck{projectDir: t.TempDir()}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)

	assert.Contains(t, res.Summary, "no linked state")
}
