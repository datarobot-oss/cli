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

package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 2 pairs the walker with the ignore matcher, and the state directory is
// only kept out of a sync by that pairing. This exercises the pair against a
// real filesystem rather than synthetic paths, because whether the leak below
// is reachable at all is a property of the filesystem, not of the matcher.
//
// A project that already holds a differently-cased .Datarobot/ gets the state
// directory created inside it on macOS and Windows: the CLI asks for
// .datarobot/workload/ and the filesystem, case-insensitive but case
// preserving, hands back the existing entry. The walk then reports
// .Datarobot/workload/config.json, which an exact-match exclude does not catch.
// Verified: without case folding this test yields that config.json.
//
// On a case-sensitive filesystem the two directories stay distinct and the
// assertion holds for the ordinary reason. Either way, no state file syncs.
func TestGatherNeverWalksStateFiles_WhateverTheCasing(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(root, ".Datarobot"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, wapi.RootDirName, wapi.StateDirName), 0o755))

	stateDir := wapi.Dir(root)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "config.json"), []byte(`{"artifactId":"a"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "manifest.json"), []byte(`{"version":1}`), 0o600))

	require.NoError(t, os.WriteFile(filepath.Join(root, "agent.py"), []byte("print('hi')\n"), 0o600))

	entries, err := fileops.Walk(root, ignore.FromLines(nil).Match, func(string, string) {})
	require.NoError(t, err)

	rels := make([]string, 0, len(entries))
	for _, e := range entries {
		rels = append(rels, e.RelPath)
	}

	assert.Equal(t, []string{"agent.py"}, rels, "only the user's own files are gathered")
}
