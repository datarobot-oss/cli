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

package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusServer answers every request with status.
func statusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	return server
}

// Callers classify on the status, so collapsing every non-200 into one error
// would leave them unable to tell a rejected token from a failing instance.
func TestVerifyTokenPreservesStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := statusServer(t, status)

			err := VerifyToken(context.Background(), server.URL+"/api/v2", "some-token")

			var statusErr *HTTPStatusError

			require.ErrorAs(t, err, &statusErr)
			assert.Equal(t, status, statusErr.StatusCode)
		})
	}
}

func TestVerifyTokenOKReturnsNil(t *testing.T) {
	server := statusServer(t, http.StatusOK)

	require.NoError(t, VerifyToken(context.Background(), server.URL+"/api/v2", "some-token"))
}

// GetAPIKey wraps VerifyToken for `dr auth check` and the request-time token
// cache; the typed error must reach them intact.
func TestGetAPIKeyPropagatesStatus(t *testing.T) {
	server := statusServer(t, http.StatusServiceUnavailable)

	viper.Set(DataRobotURL, server.URL+"/api/v2")
	viper.Set(DataRobotAPIKey, "some-token")

	t.Cleanup(viper.Reset)

	key, err := GetAPIKey(context.Background())
	assert.Empty(t, key)

	var statusErr *HTTPStatusError

	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusServiceUnavailable, statusErr.StatusCode)
}
