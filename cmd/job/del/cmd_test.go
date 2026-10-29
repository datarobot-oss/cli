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
	"bytes"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelete_WithYesDeletes(t *testing.T) {
	var deleted bool

	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/", r.URL.Path)

		deleted = true

		w.WriteHeader(http.StatusNoContent)
	})

	var out bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"job-1", "--yes"})

	require.NoError(t, cmd.Execute())
	assert.True(t, deleted)
	assert.Contains(t, out.String(), "Deleted job job-1.")
}

func TestDelete_WithoutAnAnswerDeletesNothing(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	jobtest.Serve(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request may be sent")
	})

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"job-1", "--output-format", "json"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --yes")
}
