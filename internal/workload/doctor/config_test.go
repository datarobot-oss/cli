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

func TestConfigCheck_OK(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	res := (&configCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	assert.Empty(t, res.Remedy)
}

func TestConfigCheck_FAIL_Missing(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	res := (&configCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	assert.Contains(t, res.Summary, "missing")

	assert.Equal(t, RemedyConfig, res.Remedy)

	wantPath, err := filepath.Abs(wapi.ConfigPath(dir))

	require.NoError(t, err)

	assert.Equal(t, wantPath, res.Details["path"])
}

func TestConfigCheck_FAIL_CorruptShowsAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	writeStateFile(t, dir, "config.json", `{"artifactId":"abc`)

	res := (&configCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	wantPath, err := filepath.Abs(wapi.ConfigPath(dir))

	require.NoError(t, err)

	assert.Equal(t, wantPath, res.Details["path"])

	assert.Contains(t, res.Summary, wantPath)
}

func TestConfigCheck_FAIL_SemanticValidation(t *testing.T) {
	dir := t.TempDir()

	// Parses fine but fails coupled-field validation: lastSyncedVersionId
	// set while catalogId is null.
	writeStateFile(t, dir, "config.json",
		`{"artifactId":"`+testArtifactID+`","lastSyncedVersionId":"`+testVersionID+`","createdAt":"2026-01-01T00:00:00Z","cliVersion":"test-version"}`)

	res := (&configCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	wantPath, err := filepath.Abs(wapi.ConfigPath(dir))

	require.NoError(t, err)

	assert.Equal(t, wantPath, res.Details["path"])
}

func TestConfigCheck_SKIP_NotLinked(t *testing.T) {
	res := (&configCheck{projectDir: t.TempDir()}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)

	assert.Contains(t, res.Summary, "no linked state")
}
