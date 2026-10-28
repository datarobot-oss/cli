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

package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubList(t *testing.T, fn func(id string, limit int, filter workload.EventFilter) ([]workload.WorkloadEvent, error)) {
	t.Helper()

	prev := listEventsFn
	listEventsFn = fn

	t.Cleanup(func() { listEventsFn = prev })
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs(args)

	var out bytes.Buffer

	cmd.SetOut(&out)

	err := cmd.Execute()

	return out.String(), err
}

func TestCmd_ArgIsOptional(t *testing.T) {
	// Args is called directly: with the id optional, cmd.Execute() would run
	// PreRunE and start a real authentication flow.
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

// The flags reach the client as one filter and the id as typed; the table
// carries the platform's message.
func TestCmd_PassesTheFilterAndRendersATable(t *testing.T) {
	var (
		gotID     string
		gotLimit  int
		gotFilter workload.EventFilter
	)

	stubList(t, func(id string, limit int, filter workload.EventFilter) ([]workload.WorkloadEvent, error) {
		gotID, gotLimit, gotFilter = id, limit, filter

		return []workload.WorkloadEvent{{
			Timestamp: time.Date(2026, 9, 30, 18, 10, 8, 0, time.UTC),
			EventType: "Replacement Errored",
			ActorID:   "683e3b2eb7aefb434d49763b",
			Details:   json.RawMessage(`{"message":"Failed to launch candidate protons: no bundle fits"}`),
		}}, nil
	})

	out, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0", "--type", "errored", "--type", "Completed",
		"--since", "2026-09-30", "--until", "2026-09-30", "--proton-id", "p1", "--limit", "5")
	require.NoError(t, err)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", gotID)
	assert.Equal(t, 5, gotLimit)
	assert.Equal(t, []string{"errored", "Completed"}, gotFilter.Types)
	assert.Equal(t, "p1", gotFilter.ProtonID)
	assert.Equal(t, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), gotFilter.Since)
	// The same date on both sides is the whole of that day, not an empty window.
	assert.Equal(t, time.Date(2026, 9, 30, 23, 59, 59, 999_999_999, time.UTC), gotFilter.Until)

	assert.Contains(t, out, "Replacement Errored")
	assert.Contains(t, out, "no bundle fits")
	assert.Contains(t, out, "2026-09-30 18:10 UTC")
}

func TestCmd_JSONIsOneEnvelope(t *testing.T) {
	stubList(t, func(string, int, workload.EventFilter) ([]workload.WorkloadEvent, error) {
		return nil, nil
	})

	out, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	assert.Equal(t, []any{}, envelope["events"])
}

func TestCmd_EmptyTrailSaysSo(t *testing.T) {
	stubList(t, func(string, int, workload.EventFilter) ([]workload.WorkloadEvent, error) {
		return nil, nil
	})

	out, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0")
	require.NoError(t, err)
	assert.Equal(t, "No events found.\n", out)
}

// A bad flag is refused before the client is called and names the flag.
func TestCmd_RefusesBadFlags(t *testing.T) {
	stubList(t, func(string, int, workload.EventFilter) ([]workload.WorkloadEvent, error) {
		t.Fatal("the client must not be called")

		return nil, nil
	})

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"since", []string{"--since", "yesterday"}, "--since: invalid time"},
		{"until", []string{"--until", "1y"}, "--until: invalid time"},
		{"inverted window", []string{"--since", "2026-09-30", "--until", "2026-09-29"}, "is before --since"},
		{"limit", []string{"--limit", "0"}, "limit"},
		{"output format", []string{"--output-format", "yaml"}, `invalid output format "yaml"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, append([]string{"68b0c1d2e3f4a5b6c7d8e9f0"}, tc.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestCmd_ClientErrorsSurface(t *testing.T) {
	stubList(t, func(string, int, workload.EventFilter) ([]workload.WorkloadEvent, error) {
		return nil, errors.New("boom")
	})

	_, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// A 404 for an id that came from .datarobot.yaml is explained in terms of
// the manifest, since the same file read against the wrong instance answers
// exactly this way; a typed id gets the error as it is.
func TestCmd_WrapsANotFoundForAManifestWorkload(t *testing.T) {
	stubList(t, func(string, int, workload.EventFilter) ([]workload.WorkloadEvent, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "https://example.test/api/v2/workloads/68b0c1d2e3f4a5b6c7d8e9f0/events/"}
	})

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".datarobot.yaml"), []byte("workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n"), 0o600))

	_, err := run(t, "--dir", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not on this instance")
	assert.Contains(t, err.Error(), ".datarobot.yaml")

	_, err = run(t, "68b0c1d2e3f4a5b6c7d8e9f0")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "is not on this instance", "a typed id is not explained by a manifest")
}

func TestCmd_HasNoWaitFlag(t *testing.T) {
	cmd := Cmd()

	assert.Nil(t, cmd.Flag("wait"))
	assert.Nil(t, cmd.Flag("follow"))
	assert.Nil(t, cmd.Flag("offset"))
}
