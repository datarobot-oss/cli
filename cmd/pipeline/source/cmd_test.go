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

package source

import (
	"errors"
	"io"
	"net/http"
	"testing"

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

func TestCmd_RequiresPipelineFlag(t *testing.T) {
	err := runCmd(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline")
}

func TestCmd_RejectsInvalidOutputFormat(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--output-format", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"pipeline", "output-format", "scope", "version"} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected --%s flag to be registered", name)
	}
}

func TestHandleSourceError_TextFormat_404_PrintsMessageAndReturnsNil(t *testing.T) {
	err := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "/test", Detail: "not found"}

	result := handleSourceError(err, "p-123", outputformat.OutputFormatText)

	assert.NoError(t, result)
}

func TestHandleSourceError_JSONFormat_404_ReturnsError(t *testing.T) {
	err := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "/test", Detail: "not found"}

	result := handleSourceError(err, "p-123", outputformat.OutputFormatJSON)

	require.Error(t, result)
	assert.Equal(t, err, result)
}

func TestHandleSourceError_NonNotFound_AlwaysReturnsError(t *testing.T) {
	for _, format := range []outputformat.OutputFormat{outputformat.OutputFormatText, outputformat.OutputFormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			err := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "/test", Detail: "boom"}

			result := handleSourceError(err, "p-123", format)

			require.Error(t, result)
		})
	}
}

func TestHandleSourceError_NonHTTPError_AlwaysReturnsError(t *testing.T) {
	plain := errors.New("connection refused")

	for _, format := range []outputformat.OutputFormat{outputformat.OutputFormatText, outputformat.OutputFormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			result := handleSourceError(plain, "p-123", format)

			require.Error(t, result)
			assert.Equal(t, plain, result)
		})
	}
}
