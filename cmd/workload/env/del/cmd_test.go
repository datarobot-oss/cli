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

package del

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresWorkloadIDAndAtLeastOneName(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	err := cmd.Execute()
	require.Error(t, err)
}

// TestCmd_NonexistentNameAgainstLockedArtifactNeverClones guards the fix for
// a real bug: deleting a name that isn't set used to still clone a locked
// artifact (a throwaway, unlocked draft) before discovering nothing would
// actually change, leaving pointless litter behind on every typo'd delete.
func TestCmd_NonexistentNameAgainstLockedArtifactNeverClones(t *testing.T) {
	var cloneCalled bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
			"id": "art-1",
			"status": "locked",
			"spec": {"containerGroups": [{"containers": [{
				"name": "main", "primary": true,
				"environmentVars": [{"name": "EXISTING", "value": "1"}]
			}]}]}
		}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/clone/", func(w http.ResponseWriter, _ *http.Request) {
		cloneCalled = true

		fmt.Fprint(w, `{"id":"art-2","status":"draft"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(viperx.Reset)

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "NOPE_NOT_SET"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none of the given names were set")
	assert.False(t, cloneCalled, "a guaranteed no-op delete must not clone the locked artifact")
}

// TestCmd_ActiveReplacementBlocksBeforeResolvingWorkload guards the upfront
// fail-fast check: if a replacement is already in flight, the command must
// error out before ever fetching the workload or artifact, not just before
// the rollout at the very end.
func TestCmd_ActiveReplacementBlocksBeforeResolvingWorkload(t *testing.T) {
	var workloadFetched bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"submitted"}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		workloadFetched = true

		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(viperx.Reset)

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "SOME_VAR"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a replacement in progress")
	assert.False(t, workloadFetched, "must fail before even resolving the workload")
}

// TestCmd_StageSkipsActiveReplacementCheck guards the deliberate exception:
// --stage never touches the live rollout machinery, so it must proceed even
// while a replacement is in flight.
func TestCmd_StageSkipsActiveReplacementCheck(t *testing.T) {
	var replacementChecked bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		replacementChecked = true

		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"submitted"}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
			"id": "art-1",
			"status": "draft",
			"spec": {"containerGroups": [{"containers": [{
				"name": "main", "primary": true,
				"environmentVars": [{"name": "EXISTING", "value": "1"}]
			}]}]}
		}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(viperx.Reset)

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "EXISTING", "--stage"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.False(t, replacementChecked, "--stage must skip the in-flight-replacement check entirely")
}
