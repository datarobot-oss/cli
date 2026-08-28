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
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedArtifacts is the one call phase 1 makes, answering with an artifact
// nothing can be pushed into.
type lockedArtifacts struct{}

func (lockedArtifacts) Get(id string) (*workload.Artifact, error) {
	return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
}

func (lockedArtifacts) PatchCodeRef(_, _, _ string) error { return nil }

// A locked artifact refuses the sync, and the refusal has to name a command.
// It used to end at deleting the state directory, which takes the code catalog
// and the last-synced version along with the one field being changed.
func TestPhase1Gather_LockedArtifactNamesACommandNotADirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68b0aaaa0000000000000001"}))

	e, err := newWithDeps(dir, Options{}, Deps{Artifacts: lockedArtifacts{}})
	require.NoError(t, err)

	err = phase1Gather(e)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "is locked (immutable)")
	assert.Contains(t, err.Error(), "dr artifact code init --force",
		"the way out has to be a command the reader can run")
	assert.NotContains(t, err.Error(), "delete "+wapi.Dir(dir),
		"recovery must never end at deleting the state directory")
}

// A preview is not a write, so it reports the lock rather than refusing: the
// deploy path asks for this diff before deciding to mint a new version.
func TestPhase1Gather_LockedArtifactStillPreviews(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68b0aaaa0000000000000001"}))

	e, err := newWithDeps(dir, Options{DryRun: true}, Deps{Artifacts: lockedArtifacts{}})
	require.NoError(t, err)

	require.NoError(t, phase1Gather(e))
	assert.Contains(t, e.lockedNote, "preview only")
}
