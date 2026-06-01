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
	"errors"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeleteError_404IsSuppressed(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "http://x/api/v2/pipelines/abc"}

	err := handleDeleteError(httpErr, "abc")
	assert.NoError(t, err)
}

func TestHandleDeleteError_OtherStatusesPropagate(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "http://x/api/v2/pipelines/abc"}

	err := handleDeleteError(httpErr, "abc")
	require.Error(t, err)

	var got *drapi.HTTPError

	require.ErrorAs(t, err, &got)
	assert.Equal(t, http.StatusInternalServerError, got.StatusCode)
}

func TestHandleDeleteError_NonHTTPError(t *testing.T) {
	err := handleDeleteError(errors.New("network down"), "abc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
}

func TestCmd_RegistersExpectedShape(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "delete", cmd.Name())
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.PreRunE)
}
