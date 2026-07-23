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
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/pipeline/internal/testutil"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
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

func TestCmd_RequiresImageArg(t *testing.T) {
	err := runCmd(t)
	require.Error(t, err)
}

func TestCmd_RejectsInvalidOutputFormat(t *testing.T) {
	err := runCmd(t, "--output-format", "yaml", "img-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()
	assert.NotNil(t, cmd.Flags().Lookup("output-format"), "expected --output-format flag")
}

func TestCmd_RejectsExtraArgs(t *testing.T) {
	err := runCmd(t, "img-1", "img-2")
	require.Error(t, err)
}

func TestHandleImageError_TextFormat_404_PrintsMessageAndReturnsNil(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleImageError(httpErr, "img-1", outputformat.OutputFormatText)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "No image found: img-1")
}

func TestHandleImageError_JSONFormat_404_ReturnsErrorAndKeepsStdoutClean(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleImageError(httpErr, "img-1", outputformat.OutputFormatJSON)
		require.Error(t, err)
		assert.Same(t, httpErr, err)
	})

	assert.Empty(t, output, "JSON mode must not write friendly message to stdout")
}

func TestHandleImageError_NonNotFound_Propagates(t *testing.T) {
	other := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "x"}

	err := handleImageError(other, "img-1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Same(t, other, err)
}
