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

package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sha256Fingerprint(token string) string {
	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}

func TestGetOrCreateUserID_FreshAPIUID(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/account/info/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"fresh-uid","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "fresh-uid", result)

	cachePath := filepath.Join(tmpDir, "datarobot", userIDFileName)
	data, err := os.ReadFile(cachePath)

	require.NoError(t, err)

	var cached cachedUserID

	err = json.Unmarshal(data, &cached)

	require.NoError(t, err)
	assert.Equal(t, "fresh-uid", cached.UID)
	assert.Equal(t, server.URL, cached.Endpoint)
	assert.Equal(t, sha256Fingerprint("test-token"), cached.TokenFingerprint)
}

func TestGetOrCreateUserID_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"test-uid","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	_, _ = retrieveUserID(context.Background())

	cachePath := filepath.Join(tmpDir, "datarobot", userIDFileName)
	info, err := os.Stat(cachePath)

	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode()&os.ModePerm)
}

func TestGetOrCreateUserID_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, "https://test.example.com/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	cached := cachedUserID{
		UID:              "cached-uid-123",
		Endpoint:         "https://test.example.com",
		TokenFingerprint: sha256Fingerprint("test-token"),
	}
	data, err := json.Marshal(cached)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), data, 0o600)

	require.NoError(t, err)

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "cached-uid-123", result)
}

func TestGetOrCreateUserID_CacheMiss_EndpointChanged(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"new-uid-456","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	cached := cachedUserID{
		UID:              "cached-uid-123",
		Endpoint:         "https://old.example.com",
		TokenFingerprint: sha256Fingerprint("test-token"),
	}
	data, err := json.Marshal(cached)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), data, 0o600)

	require.NoError(t, err)

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-uid-456", result)
}

func TestGetOrCreateUserID_CacheMiss_TokenChanged(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"new-uid-789","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "new-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	cached := cachedUserID{
		UID:              "cached-uid-123",
		Endpoint:         server.URL,
		TokenFingerprint: sha256Fingerprint("old-token"),
	}
	data, err := json.Marshal(cached)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), data, 0o600)

	require.NoError(t, err)

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-uid-789", result)
}

func TestGetOrCreateUserID_CacheMiss_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"api-uid-000","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "api-uid-000", result)
}

func TestGetOrCreateUserID_CacheMiss_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"recovery-uid","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), []byte("not json"), 0o600)

	require.NoError(t, err)

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "recovery-uid", result)
}

func TestGetOrCreateUserID_TokenChange_UpdatesCache(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"new-token-uid","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "new-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "new-token")

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	stale := cachedUserID{
		UID:              "stale-uid",
		Endpoint:         server.URL,
		TokenFingerprint: sha256Fingerprint("old-token"),
	}
	data, err := json.Marshal(stale)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), data, 0o600)

	require.NoError(t, err)

	result, err := retrieveUserID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-token-uid", result)

	updatedData, err := os.ReadFile(filepath.Join(configDir, userIDFileName))

	require.NoError(t, err)

	var updated cachedUserID

	err = json.Unmarshal(updatedData, &updated)

	require.NoError(t, err)
	assert.Equal(t, "new-token-uid", updated.UID)
	assert.Equal(t, server.URL, updated.Endpoint)
	assert.Equal(t, sha256Fingerprint("new-token"), updated.TokenFingerprint)
}

func TestRetrieveUserID_APIFailureReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	server.Close()

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	result, err := retrieveUserID(context.Background())

	require.Error(t, err)
	assert.Empty(t, result)
}

func TestGetUserID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/account/info/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uid":"test-uid-123","email":"user@example.com"}`))
	}))
	defer server.Close()

	testutil.StubAPIToken(t, "test-token")

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

	testutil.StubAPIToken(t, "test-token")

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

	testutil.StubAPIToken(t, "test-token")

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

	testutil.StubAPIToken(t, "test-token")

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	uid, err := GetUserID(context.Background())
	require.Error(t, err)
	assert.Empty(t, uid)
}
