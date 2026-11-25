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

func TestPresenceCheck_OK_CurrentLocation(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	res := (&presenceCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	assert.Contains(t, res.Summary, "linked")

	assert.Empty(t, res.Remedy)
}

func TestPresenceCheck_OK_LegacyLocation(t *testing.T) {
	dir := t.TempDir()

	// Legacy-only project: no .datarobot/workload, just .wapi.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, wapi.LegacyDirName), 0o755))

	res := (&presenceCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	assert.Contains(t, res.Summary, "linked")
}

func TestPresenceCheck_FAIL_NotLinked(t *testing.T) {
	res := (&presenceCheck{projectDir: t.TempDir()}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	assert.Equal(t, RemedyPresence, res.Remedy)

	assert.Contains(t, res.Summary, "not linked")
}

func TestPresenceCheck_FAIL_StateDirPathIsFile(t *testing.T) {
	dir := t.TempDir()

	// A regular file where the state dir belongs must not be treated as
	// linked (and must not crash the check).
	statePath := filepath.Join(dir, wapi.RootDirName)

	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))

	require.NoError(t, os.WriteFile(statePath, []byte("not a directory"), 0o600))

	res := (&presenceCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)
}
