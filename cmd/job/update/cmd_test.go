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

package update

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func run(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestUpdate_NeedsSomethingToChange(t *testing.T) {
	err := run(t, "job-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to update")
}

func TestUpdate_SendsOnlyWhatWasGiven(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/", r.URL.Path)
		assert.Equal(t, map[string]any{"artifactId": "art-2"}, jobtest.Body(t, r))
		fmt.Fprint(w, jobtest.JobDoc)
	})

	require.NoError(t, run(t, "job-1", "--artifact-id", "art-2"))
}

func TestUpdate_NeedsAJobID(t *testing.T) {
	err := run(t, "--name", "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
