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

package userid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/account/info/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"test-uid-123","email":"user@example.com"}`))
	}))
	defer server.Close()

	defer resetTokenForTest(t, "test-token")()
	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	uid, err := GetUserID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-uid-123", uid)
}

func TestGetUserID_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	defer resetTokenForTest(t, "test-token")()
	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	uid, err := GetUserID(context.Background())
	require.Error(t, err)
	assert.Empty(t, uid)
}

func TestGetUserID_EmptyUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/account/info/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"","email":"user@example.com"}`))
	}))
	defer server.Close()

	defer resetTokenForTest(t, "test-token")()
	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	uid, err := GetUserID(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty uid")
	assert.Empty(t, uid)
}

func TestGetUserID_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Should never be called because we close the server before making the request.
	}))
	server.Close()

	defer resetTokenForTest(t, "test-token")()
	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	uid, err := GetUserID(context.Background())
	require.Error(t, err)
	assert.Empty(t, uid)
}

func resetTokenForTest(t *testing.T, token string) func() {
	original := drapi.GetToken()
	drapi.SetToken(token)
	return func() {
		drapi.SetToken(original)
	}
}
