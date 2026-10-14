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

package codesync

import (
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Under --push-only a --verify run with nothing to push still lists what it
// left alone before saying the record is being repaired, and the summary
// says those paths keep their old record.
func TestRunE_PushOnlyVerify_EmptyPlanListsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{
			Skipped: []sync.FileAction{{Path: "stray.py", Classification: sync.ClsRemoteAdded}},
		},
		divergences: []sync.Divergence{
			{Path: "stray.py", Kind: sync.DivergenceRemoteOnly, RemoteHash: "bbbb"},
		},
		result: &sync.Result{NewVersion: "v1"},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true", "push-only": "true"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.True(t, fe.executed, "the record is still rewritten for the paths not left alone")
	assert.Contains(t, stdout.String(), "LEFT ALONE")
	assert.Contains(t, stdout.String(), "stray.py")
	assert.Contains(t, stdout.String(), "rewritten from the server's state")
	assert.NotContains(t, stdout.String(), "Up to date.")
	assert.Contains(t, stderr.String(), "except for the paths left alone")
}

// The engine the command builds is quiet: the findings are summarised once
// from here, not logged from the phases as well.
func TestRunE_EngineIsQuiet(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	var seen sync.Options

	deps := fakeEngineDeps(&fakeEngine{plan: &sync.SyncPlan{}})
	inner := deps.NewEngine
	deps.NewEngine = func(dir string, opts sync.Options) (engineRunner, error) {
		seen = opts

		return inner(dir, opts)
	}

	_, _, _, err := runWithDeps(t, deps, map[string]string{"dir": dir, "yes": "true"})
	require.NoError(t, err)
	assert.True(t, seen.Quiet)
}
