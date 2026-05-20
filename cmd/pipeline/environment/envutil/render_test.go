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

package envutil

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout redirects os.Stdout, runs fn, and returns the captured
// output. The previous stdout is restored unconditionally.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	prev := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	defer func() {
		os.Stdout = prev
	}()

	fn()

	_ = w.Close()

	buf, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(buf)
}

func TestPrintEnvironmentJSON(t *testing.T) {
	desc := "demo"
	env := pipeline.Environment{
		EnvironmentID: "env-1",
		Name:          "ml-base",
		Description:   &desc,
		LatestVersion: 1,
	}

	out := captureStdout(t, func() {
		require.NoError(t, PrintEnvironmentJSON(env))
	})

	assert.Contains(t, out, `"environment_id": "env-1"`)
	assert.Contains(t, out, `"name": "ml-base"`)
}

func TestPrintEnvironmentHuman_RendersVersionsTable(t *testing.T) {
	env := pipeline.Environment{
		EnvironmentID: "env-1",
		Name:          "ml-base",
		LatestVersion: 2,
		Versions: []pipeline.EnvironmentVersion{
			{Version: 2, Packages: []string{"scikit-learn"}, Status: pipeline.EnvironmentStatusReady, UpdatedAt: "u2"},
			{Version: 1, Packages: []string{"numpy", "pandas"}, Status: pipeline.EnvironmentStatusReady, UpdatedAt: "u1"},
		},
	}

	out := captureStdout(t, func() {
		PrintEnvironmentHuman(env)
	})

	assert.Contains(t, out, "Environment ID: env-1")
	assert.Contains(t, out, "Latest version: v2")
	assert.Contains(t, out, "VERSION")
	assert.Contains(t, out, "v2")
	assert.Contains(t, out, "scikit-learn")
}

func TestPrintEnvironmentListHuman_EmptyAndPopulated(t *testing.T) {
	out := captureStdout(t, func() {
		PrintEnvironmentListHuman(nil)
	})
	assert.Contains(t, out, "No environments found")

	out = captureStdout(t, func() {
		PrintEnvironmentListHuman([]pipeline.EnvironmentSummary{
			{EnvironmentID: "env-1", Name: "ml-base", LatestVersion: 3, LatestStatus: pipeline.EnvironmentStatusReady, UpdatedAt: "u"},
		})
	})
	assert.Contains(t, out, "ENVIRONMENT_ID")
	assert.Contains(t, out, "env-1")
	assert.Contains(t, out, "v3")
}

func TestPrintEnvironmentListJSON_EmitsArray(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, PrintEnvironmentListJSON([]pipeline.EnvironmentSummary{
			{EnvironmentID: "env-1", Name: "n", LatestVersion: 1, LatestStatus: pipeline.EnvironmentStatusReady},
		}))
	})

	// Sanity check JSON parseability via simple substring assertions; the
	// real JSON shape is exercised by the shared json.MarshalIndent path.
	assert.Contains(t, out, `[`)
	assert.Contains(t, out, `]`)
	assert.Contains(t, out, `"environment_id": "env-1"`)
}

func TestJoinPackages_Truncates(t *testing.T) {
	long := bytes.Repeat([]byte("x"), 80)
	out := joinPackages([]string{string(long)})
	assert.Len(t, out, 60)
	assert.True(t, len(out) > 3 && out[len(out)-3:] == "...")
}
