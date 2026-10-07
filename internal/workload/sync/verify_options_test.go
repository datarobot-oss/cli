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
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// previewOnly answers exactly one question: does this run stop after Plan?
// Verify changes how much a run checks, not whether it applies its plan, so
// folding it into this predicate would silently turn every verify user's
// sync into a preview. These cases pin the boundary.
func TestPreviewOnly_DoesNotConsiderVerify(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
		want bool
	}{
		{"no flags", Options{}, false},
		{"verify only", Options{Verify: true}, false},
		{"dry run", Options{DryRun: true}, true},
		{"diff", Options{ShowDiffs: true}, true},
		{"verify alongside dry run is still a preview", Options{Verify: true, DryRun: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Engine{opts: tc.opts}

			assert.Equal(t, tc.want, e.previewOnly())
		})
	}
}

// TestEngine_Run_VerifyStillExecutes: Run() under Options{Verify: true}
// must still apply the plan and create a version, because previewOnly does
// not consider Verify. This is the behavioural form of the boundary above —
// the moment someone folds Verify into previewOnly, Run returns the empty
// pre-plan result and this fails.
func TestEngine_Run_VerifyStillExecutes(t *testing.T) {
	dir := initProject(t, map[string]string{"agent.py": "print('hi')\n"})

	fake := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	e, err := newWithDeps(dir, Options{Verify: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ver-1", result.NewVersion, "a verify-only run must still create a version")
	assert.Equal(t, 2, result.UploadedCount, "expect agent.py + .drignore")
}
