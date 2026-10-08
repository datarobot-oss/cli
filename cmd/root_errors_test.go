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

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorEnvelope struct {
	Error map[string]any `json:"error"`
}

// runFailing executes a tree whose "stub" command fails with runErr, and
// returns what reached stdout and stderr.
func runFailing(t *testing.T, runErr error, args []string, opts ...Option) (string, string) {
	t.Helper()
	t.Setenv(outputFormatEnvVar, "")

	root := NewIsolatedRootFactory(opts...).Build()

	stub := &cobra.Command{
		Use:  "stub",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return runErr },
	}

	root.AddCommand(stub)

	var stdout, stderr bytes.Buffer

	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := executeRoot(context.Background(), root.Command, args)
	require.Error(t, err)

	return stdout.String(), stderr.String()
}

func decodeEnvelope(t *testing.T, stderr string) map[string]any {
	t.Helper()

	var env errorEnvelope

	require.NoError(t, json.Unmarshal([]byte(stderr), &env), "stderr must be one JSON value: %q", stderr)
	require.NotNil(t, env.Error)

	return env.Error
}

func TestExecuteRoot_JSONPlainErrorWritesEnvelope(t *testing.T) {
	stdout, stderr := runFailing(t, errors.New("boom"), []string{"stub", "--output-format", "json"})

	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "Error:")
	assert.JSONEq(t, `{"error":{"message":"boom"}}`, stderr)
	assert.Equal(t, 1, strings.Count(stderr, "\n"), "the envelope is one line")
}

func TestExecuteRoot_JSONHTTPErrorCarriesStatusAndURL(t *testing.T) {
	httpErr := &drapi.HTTPError{
		StatusCode: 422,
		URL:        "https://app.example.com/api/v2/workloads/?limit=1&offset=0",
		Detail:     "name is required",
	}

	stdout, stderr := runFailing(t, fmt.Errorf("create workload: %w", httpErr), []string{"--output-format=json", "stub"})

	assert.Empty(t, stdout)

	body := decodeEnvelope(t, stderr)

	assert.Equal(t, "create workload: "+httpErr.Error(), body["message"])
	assert.InDelta(t, 422, body["statusCode"], 0)
	assert.Equal(t, httpErr.URL, body["url"])
	assert.Equal(t, "name is required", body["detail"])
}

func TestExecuteRoot_TextModeUnchanged(t *testing.T) {
	stdout, stderr := runFailing(t, errors.New("boom"), []string{"stub"})

	assert.Empty(t, stdout)
	assert.Equal(t, "Error: boom\n", stderr)
}

func TestExecuteRoot_ErrSilentGetsNoEnvelope(t *testing.T) {
	t.Setenv(outputFormatEnvVar, "")

	root := NewIsolatedRootFactory().Build()

	stub := &cobra.Command{
		Use:           "stub",
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), `{"status":"failed"}`)

			return cli.ErrSilent
		},
	}

	root.AddCommand(stub)

	var stdout, stderr bytes.Buffer

	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := executeRoot(context.Background(), root.Command, []string{"stub", "--output-format", "json"})

	require.ErrorIs(t, err, cli.ErrSilent)
	assert.JSONEq(t, `{"status":"failed"}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestExecuteRoot_JSONUsageErrorsWriteEnvelope(t *testing.T) {
	cases := map[string]struct {
		args    []string
		message string
	}{
		"unknown flag after format": {
			args:    []string{"stub", "--output-format", "json", "--bogus"},
			message: "unknown flag: --bogus",
		},
		"unknown flag before format": {
			args:    []string{"stub", "--bogus", "--output-format", "json"},
			message: "unknown flag: --bogus",
		},
		"unexpected argument": {
			args:    []string{"stub", "extra", "--output-format=json"},
			message: `unknown command "extra" for "dr stub"`,
		},
		"unknown root flag": {
			args:    []string{"--bogus", "--output-format", "json", "stub"},
			message: "unknown flag: --bogus",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stdout, stderr := runFailing(t, nil, tc.args)

			assert.Empty(t, stdout)
			assert.Equal(t, tc.message, decodeEnvelope(t, stderr)["message"])
		})
	}
}

func TestExecuteRoot_JSONFromEnvVar(t *testing.T) {
	t.Setenv(outputFormatEnvVar, "json")

	root := NewIsolatedRootFactory().Build()
	root.AddCommand(&cobra.Command{
		Use:  "stub",
		RunE: func(_ *cobra.Command, _ []string) error { return errors.New("boom") },
	})

	var stderr bytes.Buffer

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)

	require.Error(t, executeRoot(context.Background(), root.Command, []string{"stub"}))
	assert.Equal(t, "boom", decodeEnvelope(t, stderr.String())["message"])
}

func TestExecuteRoot_JSONFromConfigFile(t *testing.T) {
	t.Cleanup(func() { viperx.Set("output-format", "") })

	stdout, stderr := runFailing(t, errors.New("boom"), []string{"stub"},
		WithConfigInitializer(func(_ *cobra.Command) error {
			viperx.Set("output-format", "json")

			return nil
		}),
	)

	assert.Empty(t, stdout)
	assert.Equal(t, "boom", decodeEnvelope(t, stderr)["message"])
}

func TestExecuteRoot_ExplicitTextFlagOverridesEnv(t *testing.T) {
	assert.False(t, jsonRequestedWithEnv(t, "json", []string{"stub", "--output-format", "text"}))
	assert.True(t, jsonRequestedWithEnv(t, "json", []string{"stub"}))
	assert.False(t, jsonRequestedWithEnv(t, "", []string{"stub", "--", "--output-format", "json"}))
}

func jsonRequestedWithEnv(t *testing.T, env string, args []string) bool {
	t.Helper()
	t.Setenv(outputFormatEnvVar, env)

	return jsonRequested(args)
}
