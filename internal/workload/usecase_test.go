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
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadUseCases_ReadsOneFilteredPage(t *testing.T) {
	var calls atomic.Int32

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/useCases/", r.URL.Path)
		assert.Equal(t, "wl-1", r.URL.Query().Get("entityId"))
		assert.Equal(t, "workload", r.URL.Query().Get("entityType"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))

		// The next link drops the filter, which is why it is never followed.
		fmt.Fprint(w, `{"data":[{"id":"uc-1"},{"id":"uc-2"}],"next":"http://elsewhere/api/v2/useCases/?offset=100"}`)
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Equal(t, []string{"uc-1", "uc-2"}, links.IDs)
	assert.True(t, links.More)
	assert.True(t, links.Has("uc-2"))
	assert.False(t, links.Has("uc-3"))
	assert.Equal(t, int32(1), calls.Load(), "one page only")
}

func TestWorkloadUseCases_NoNextMeansComplete(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[],"next":null}`)
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Empty(t, links.IDs)
	assert.False(t, links.More)
}

func TestLinkWorkloadToUseCase_PostsTheLinkRoute(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/useCases/uc-1/workloads/wl-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, LinkWorkloadToUseCase("uc-1", "wl-1"))
}

func TestLinkWorkloadToUseCase_AlreadyLinkedIsSuccess(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Entity is already linked to this Use Case"}`)
	}))

	require.NoError(t, LinkWorkloadToUseCase("uc-1", "wl-1"))
}

func TestLinkWorkloadToUseCase_RefusalKeepsTheStatus(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail":{"code":"ENCLAVE_TARGETING_REQUIRED","message":"Set a selection policy"}}`)
	}))

	err := LinkWorkloadToUseCase("uc-1", "wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "ENCLAVE_TARGETING_REQUIRED")
}

func TestUnlinkWorkloadFromUseCase_DeletesTheLinkRoute(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/useCases/uc-1/workloads/wl-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, UnlinkWorkloadFromUseCase("uc-1", "wl-1"))
}

func TestUseCaseLinkRequest_NeedsBothIDs(t *testing.T) {
	require.Error(t, LinkWorkloadToUseCase("", "wl-1"))
	require.Error(t, UnlinkWorkloadFromUseCase("uc-1", " "))
}
