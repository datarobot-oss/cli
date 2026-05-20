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

package get

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RejectsMissingPipelineFlag(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"2"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--pipeline")
}

func TestCmd_RejectsNonNumericVersion(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"abc", "--pipeline", "p"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestCmd_RejectsZeroOrNegativeVersion(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"0", "--pipeline", "p"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestHandleGetError_404IsSuppressed(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	err := handleGetError(httpErr, "2")
	assert.NoError(t, err)
}

func TestHandleGetError_OtherErrorsPropagate(t *testing.T) {
	err := handleGetError(errors.New("boom"), "2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
