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

package runutil

import (
	"errors"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/pipeline/internal/testutil"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRunNotFoundError_404IsSuppressed(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}
	assert.NoError(t, HandleRunNotFoundError(httpErr, "d-1", outputformat.OutputFormatText))
}

func TestHandleRunNotFoundError_PropagatesOther(t *testing.T) {
	err := HandleRunNotFoundError(errors.New("boom"), "d-1", outputformat.OutputFormatText)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestHandleRunNotFoundError_TextFormat_404_PrintsMessageAndReturnsNil(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := HandleRunNotFoundError(httpErr, "d-1", outputformat.OutputFormatText)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "No run found with id: d-1")
}

func TestHandleRunNotFoundError_JSONFormat_404_ReturnsErrorAndKeepsStdoutClean(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := HandleRunNotFoundError(httpErr, "d-1", outputformat.OutputFormatJSON)
		require.Error(t, err)
		assert.Same(t, httpErr, err)
	})

	assert.Empty(t, output, "JSON mode must not write friendly message to stdout")
}

func TestHandleRunNotFoundError_JSONFormat_NonNotFound_Propagates(t *testing.T) {
	other := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "x"}

	err := HandleRunNotFoundError(other, "d-1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Same(t, other, err)
}
