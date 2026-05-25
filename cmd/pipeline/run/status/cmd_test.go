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

package status

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCmd(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	return cmd.Execute()
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "--output-format", "yaml", "d-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_RejectsMissingPipeline(t *testing.T) {
	err := runCmd(t, "d-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--pipeline")
}

func TestHandleStatusError_404IsSuppressed(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}
	assert.NoError(t, handleStatusError(httpErr, "d-1"))
}

func TestHandleStatusError_PropagatesOther(t *testing.T) {
	err := handleStatusError(errors.New("boom"), "d-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
