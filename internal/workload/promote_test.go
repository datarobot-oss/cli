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

package workload

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoteWorkload_PostsToThePromoteRoute(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/workloads/a%2Fb/promote", r.URL.EscapedPath())

		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{}`, string(body))

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"id": "a/b", "name": "my-app", "status": "running", "artifactId": "art-1"}`)
	}))

	got, err := PromoteWorkload("a/b")
	require.NoError(t, err)
	assert.Equal(t, "my-app", got.Name)
	assert.Equal(t, "art-1", got.ArtifactID)
}

func TestPromoteWorkload_AlreadyLockedIsAnHTTPError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail": "Artifact is already locked. Only draft artifacts can be promoted."}`)
	}))

	_, err := PromoteWorkload("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "already locked")
}
