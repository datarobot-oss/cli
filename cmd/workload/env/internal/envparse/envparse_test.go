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

package envparse

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installTestServer wires viperx so config.GetEndpointURL resolves against
// srv, mirroring the pattern used by cmd/workload/env/internal/rollout's own
// test helper of the same name.
func installTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

func TestParseArg_Plain(t *testing.T) {
	ev, err := ParseArg("LOG_LEVEL=debug")
	require.NoError(t, err)
	assert.Equal(t, workload.EnvironmentVar{Name: "LOG_LEVEL", Value: "debug"}, ev)
}

func TestParseArg_PlainValueContainingEquals(t *testing.T) {
	ev, err := ParseArg("DSN=postgres://user:pass@host/db?sslmode=require")
	require.NoError(t, err)
	assert.Equal(t, "DSN", ev.Name)
	assert.Equal(t, "postgres://user:pass@host/db?sslmode=require", ev.Value)
}

func TestParseArg_Credential(t *testing.T) {
	ev, err := ParseArg("API_KEY=dr-credential:64f0abc123/apiToken")
	require.NoError(t, err)
	assert.Equal(t, workload.EnvironmentVar{
		Source:         workload.EnvironmentVarSourceDRCredential,
		Name:           "API_KEY",
		DRCredentialID: "64f0abc123",
		Key:            "apiToken",
	}, ev)
}

func TestParseArg_Errors(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		wantSub string
	}{
		{"no equals sign", "NOVALUE", "expected KEY=VALUE"},
		{"empty key", "=value", "expected KEY=VALUE"},
		{"credential missing key", "API_KEY=dr-credential:64f0abc123", "expected dr-credential"},
		{"credential missing id", "API_KEY=dr-credential:/apiToken", "expected dr-credential"},
		{"credential empty everything", "API_KEY=dr-credential:", "expected dr-credential"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseArg(c.arg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantSub)
		})
	}
}

// TestParseArg_ValidNames guards the exact k8s env-var-name rule: dashes,
// dots, and leading underscores are all legal (unusual as they are for an
// env var), only a leading digit or disallowed characters are not.
func TestParseArg_ValidNames(t *testing.T) {
	for _, name := range []string{"LOG_LEVEL", "log-level", "log.level", "_LEADING_UNDERSCORE", ".leading-dot"} {
		_, err := ParseArg(name + "=x")
		require.NoError(t, err, "name %q should be accepted", name)
	}
}

// TestParseArg_InvalidNames guards the fix for a live-verified gap: the
// platform accepts and silently stores these at PATCH time with no
// complaint, so the CLI must reject them locally instead.
func TestParseArg_InvalidNames(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"space in name", "BAD NAME=x"},
		{"leading digit", "1BAD=x"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseArg(c.arg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid environment variable name")
		})
	}
}

func TestBuildVar_ValidatesNameEvenWithoutSplitting(t *testing.T) {
	_, err := BuildVar("1BAD", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid environment variable name")
}

func TestValidateCredentialReferences_AllExist(t *testing.T) {
	var hits int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++

		fmt.Fprint(w, `{"credentialId":"cred-1","credentialType":"s3"}`)
	}))
	installTestServer(t, srv)

	err := ValidateCredentialReferences([]workload.EnvironmentVar{
		{Name: "PLAIN", Value: "x"},
		{Source: workload.EnvironmentVarSourceDRCredential, Name: "A", DRCredentialID: "cred-1", Key: "k1"},
		{Source: workload.EnvironmentVarSourceDRCredential, Name: "B", DRCredentialID: "cred-1", Key: "k2"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, hits, "the same credential id referenced twice must only be checked once")
}

func TestValidateCredentialReferences_MissingReportsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"The credential item: bogus is not found"}`)
	}))
	installTestServer(t, srv)

	err := ValidateCredentialReferences([]workload.EnvironmentVar{
		{Source: workload.EnvironmentVarSourceDRCredential, Name: "A", DRCredentialID: "bogus", Key: "k1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestValidateCredentialReferences_ServerErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	installTestServer(t, srv)

	err := ValidateCredentialReferences([]workload.EnvironmentVar{
		{Source: workload.EnvironmentVarSourceDRCredential, Name: "A", DRCredentialID: "cred-1", Key: "k1"},
	})
	require.Error(t, err, "a check failure (not a confirmed-missing 404) must not be swallowed as success")
}
