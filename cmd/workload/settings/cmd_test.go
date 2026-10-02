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

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const id = "68b0c1d2e3f4a5b6c7d8e9f0"

// stopped is the settings document staging returns for a one-group
// workload on a fixed replica count.
const stopped = `{"runtime":{"containerGroups":[{"name":"default","resourceBundles":["cpu.small"],
  "bundleSelectionPolicy":"availability","replicaCount":1,"autoscaling":null,
  "containers":[{"name":"primary","resourceAllocation":{"gpu":null,"cpu":0.5,"memory":512000000}}],
  "resolvedBundle":{"id":"cpu.small","cpuCount":1.0,"memoryBytes":536870912,"gpuCount":0,"gpuMaker":null,"gpuTypeLabel":null}}]},
  "replacement":null}`

// autoscaled is the document for a group whose count belongs to the autoscaler.
const autoscaled = `{"runtime":{"containerGroups":[{"name":"default","resourceBundles":["cpu.xlarge"],
  "bundleSelectionPolicy":"availability","replicaCount":null,
  "autoscaling":{"enabled":true,"minReplicaCount":0,"maxReplicaCount":3,
    "policies":[{"scalingMetric":"httpRequestsConcurrency","target":2.0}],"cooldownPeriodMinutes":null},
  "containers":[{"name":"primary","resourceAllocation":{"gpu":null,"cpu":1.0,"memory":2147483648}}],
  "resolvedBundle":{"id":"cpu.xlarge","cpuCount":2.0,"memoryBytes":2147483648,"gpuCount":0,"gpuMaker":null,"gpuTypeLabel":null}}]},
  "replacement":null}`

type seams struct {
	get     func(string) (*workload.WorkloadSettings, error)
	update  func(string, json.RawMessage) (*workload.Replacement, error)
	guard   func(string) error
	waitR   func(context.Context, string, *workload.Replacement, time.Duration, time.Duration, func(*workload.Replacement)) (*workload.Replacement, error)
	waitW   func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
	readDoc func(string) (json.RawMessage, error)
}

func install(t *testing.T, s seams) {
	t.Helper()

	prev := []any{getSettingsFn, updateSettingsFn, guardFn, waitReplacementFn, waitWorkloadFn, readSpecFileFn}

	t.Cleanup(func() {
		getSettingsFn = prev[0].(func(string) (*workload.WorkloadSettings, error))
		updateSettingsFn = prev[1].(func(string, json.RawMessage) (*workload.Replacement, error))
		guardFn = prev[2].(func(string) error)
		waitReplacementFn = prev[3].(func(context.Context, string, *workload.Replacement, time.Duration, time.Duration, func(*workload.Replacement)) (*workload.Replacement, error))
		waitWorkloadFn = prev[4].(func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error))
		readSpecFileFn = prev[5].(func(string) (json.RawMessage, error))
	})

	// Every seam the test did not wire fails loudly rather than reaching a
	// tenant.
	getSettingsFn = func(string) (*workload.WorkloadSettings, error) {
		t.Fatal("settings were read, which this test did not wire")

		return nil, nil
	}
	updateSettingsFn = func(string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("settings were updated, which this test did not wire")

		return nil, nil
	}
	guardFn = func(string) error { return nil }
	waitReplacementFn = func(context.Context, string, *workload.Replacement, time.Duration, time.Duration, func(*workload.Replacement)) (*workload.Replacement, error) {
		t.Fatal("a replacement was waited on, which this test did not wire")

		return nil, nil
	}
	waitWorkloadFn = func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		t.Fatal("a workload was waited on, which this test did not wire")

		return nil, nil
	}
	readSpecFileFn = workload.ReadSpecFile

	if s.get != nil {
		getSettingsFn = s.get
	}

	if s.update != nil {
		updateSettingsFn = s.update
	}

	if s.guard != nil {
		guardFn = s.guard
	}

	if s.waitR != nil {
		waitReplacementFn = s.waitR
	}

	if s.waitW != nil {
		waitWorkloadFn = s.waitW
	}

	if s.readDoc != nil {
		readSpecFileFn = s.readDoc
	}
}

func settingsDoc(t *testing.T, doc string) *workload.WorkloadSettings {
	t.Helper()

	var s workload.WorkloadSettings

	require.NoError(t, json.Unmarshal([]byte(doc), &s))

	return &s
}

func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := Cmd()

	var out, errOut bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	cmd.PreRunE = nil

	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func TestCmd_ArgIsOptional(t *testing.T) {
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{id}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

// The read: one row per container, the replicas, the bundle the platform
// resolved, and memory in the spelling a manifest uses.
func TestCmd_ShowsTheSettingsAsATable(t *testing.T) {
	install(t, seams{get: func(got string) (*workload.WorkloadSettings, error) {
		assert.Equal(t, id, got)

		return settingsDoc(t, stopped), nil
	}})

	stdout, _, err := run(t, id)
	require.NoError(t, err)

	for _, want := range []string{"GROUP", "default", "REPLICAS", "1", "cpu.small", "primary", "0.5", "512MB"} {
		assert.Contains(t, stdout, want)
	}

	assert.NotContains(t, stdout, "settings change", "nothing is in flight and nothing has finished")
}

// A change still being rolled out is printed under the table, and a finished
// one is reported as the last change rather than as one in flight.
func TestCmd_ShowsTheReplacementInFlight(t *testing.T) {
	for status, want := range map[string]string{
		"promoting": "A settings change is being rolled out: replacement rep-9 (promoting)",
		"completed": "Last settings change: replacement rep-9 (completed)",
	} {
		t.Run(status, func(t *testing.T) {
			doc := strings.Replace(stopped, `"replacement":null`,
				`"replacement":{"id":"rep-9","status":"`+status+`"}`, 1)

			install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, doc), nil }})

			stdout, _, err := run(t, id)
			require.NoError(t, err)
			assert.Contains(t, stdout, want)
		})
	}
}

// The JSON the read prints is a valid --spec-file: the envelope is unwrapped
// and the platform's resolvedBundle is not sent back.
func TestCmd_SpecFileTakesTheJSONOutput(t *testing.T) {
	install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil }})

	printed, _, err := run(t, id, "--output-format", "json")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(printed), 0o600))

	var sent json.RawMessage

	install(t, seams{update: func(_ string, runtime json.RawMessage) (*workload.Replacement, error) {
		sent = runtime

		return &workload.Replacement{ID: "rep-2", Status: "unknown"}, nil
	}})

	_, _, err = run(t, id, "--spec-file", path, "--yes")
	require.NoError(t, err)

	assert.Contains(t, string(sent), `"containerGroups"`)
	assert.Contains(t, string(sent), `"replicaCount":1`)
	assert.NotContains(t, string(sent), "resolvedBundle", "the platform's answer is not a request")
	assert.NotContains(t, string(sent), `"settings"`, "the envelope is not sent inside itself")
}

// --poll-timeout bounds the whole --wait: the second wait gets what the
// first left, floored so a spent budget still buys one look.
func TestBudgetLeft(t *testing.T) {
	assert.InDelta(t, 20*time.Minute, budgetLeft(30*time.Minute, time.Now().Add(-10*time.Minute)), float64(time.Second))
	assert.Equal(t, minPollBudget, budgetLeft(30*time.Minute, time.Now().Add(-40*time.Minute)))
}

func TestCmd_ShowsAutoscalingAndAnIndivisibleMemory(t *testing.T) {
	install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, autoscaled), nil }})

	stdout, _, err := run(t, id)
	require.NoError(t, err)
	assert.Contains(t, stdout, "auto")
	assert.Contains(t, stdout, "0-3 on httpRequestsConcurrency=2")
	assert.Contains(t, stdout, "2147483648", "2 GiB is not rounded to 2GB")
}

// JSON is one envelope carrying the platform's runtime as it came.
func TestCmd_JSONIsTheWholeOfStdout(t *testing.T) {
	install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil }})

	stdout, stderr, err := run(t, id, "--output-format", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout must be one JSON document")

	body, ok := envelope["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, id, body["workloadId"])
	assert.Nil(t, body["replacement"])

	runtime, ok := body["runtime"].(map[string]any)
	require.True(t, ok)

	groups, _ := runtime["containerGroups"].([]any)
	require.Len(t, groups, 1)

	group, _ := groups[0].(map[string]any)
	assert.Contains(t, group, "resolvedBundle", "the platform's field names survive the round trip")
}

// --replicas re-sends the whole runtime with one number changed and the
// platform's resolved bundle dropped, because the route reads the body as
// the entire runtime.
func TestCmd_ReplicasSendsTheWholeRuntime(t *testing.T) {
	var sent json.RawMessage

	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil },
		update: func(got string, runtime json.RawMessage) (*workload.Replacement, error) {
			assert.Equal(t, id, got)

			sent = runtime

			return &workload.Replacement{ID: "rep-1", WorkloadID: id, Status: "unknown"}, nil
		},
	})

	stdout, stderr, err := run(t, id, "--replicas", "3", "--yes")
	require.NoError(t, err)

	var runtime map[string]any

	require.NoError(t, json.Unmarshal(sent, &runtime))

	groups, _ := runtime["containerGroups"].([]any)
	require.Len(t, groups, 1)

	group, _ := groups[0].(map[string]any)
	assert.InDelta(t, 3, group["replicaCount"], 0)
	assert.NotContains(t, group, "resolvedBundle")
	assert.Equal(t, []any{"cpu.small"}, group["resourceBundles"], "the bundle request travels")

	containers, _ := group["containers"].([]any)
	require.Len(t, containers, 1, "the containers travel: without them the resize is to nothing")

	assert.Contains(t, stdout, "Settings update requested for workload "+id)
	assert.Contains(t, stdout, "rep-1")
	assert.Contains(t, stderr, "rolls the workload's containers", "the restart is announced even under --yes")
	assert.Contains(t, stderr, "dr workload settings "+id, "status stays running throughout; settings prints the replacement")
}

func TestCmd_ReplicasRefusesAnAutoscaledGroup(t *testing.T) {
	install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, autoscaled), nil }})

	_, _, err := run(t, id, "--replicas", "3", "--yes")
	require.ErrorIs(t, err, workload.ErrGroupAutoscales)
	assert.Contains(t, err.Error(), "--spec-file")
}

// A settings body from a file is sent as its runtime, whichever of the two
// shapes the file used.
func TestCmd_SpecFileSendsTheRuntime(t *testing.T) {
	for name, doc := range map[string]string{
		"settings body": `{"runtime": {"containerGroups": [{"name": "default", "replicaCount": 2,
			"resourceBundles": ["cpu.small"], "containers": [{"name": "primary"}]}]}}`,
		"runtime block": `containerGroups:
  - name: default
    replicaCount: 2
    containers:
      - name: primary
        resourceAllocation:
          cpu: 1
          memory: 512MB
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings")
			require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

			var sent json.RawMessage

			install(t, seams{update: func(_ string, runtime json.RawMessage) (*workload.Replacement, error) {
				sent = runtime

				return &workload.Replacement{ID: "rep-2", Status: "unknown"}, nil
			}})

			_, _, err := run(t, id, "--spec-file", path, "--yes")
			require.NoError(t, err)

			var runtime map[string]any

			require.NoError(t, json.Unmarshal(sent, &runtime))
			assert.Contains(t, runtime, "containerGroups")
			assert.NotContains(t, runtime, "runtime", "the body's wrapper is not sent inside itself")
		})
	}
}

// A body the platform would accept and then fail on is refused before the
// send, naming the group and what it lacks; nothing is confirmed or sent.
func TestCmd_SpecFileRefusesAGroupTheRolloutWouldFailOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"runtime": {"containerGroups": [{"name": "default", "replicaCount": 2}]}}`), 0o600))

	install(t, seams{})

	_, stderr, err := run(t, id, "--spec-file", path, "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "containerGroups[0] (default) needs at least one container")
	assert.NotContains(t, stderr, "rolls the workload's containers", "no restart is announced for a refused change")
}

// The flags are checked against each other before anything reaches the
// network, and the errors name the flags.
func TestCmd_RefusesInconsistentFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"replicas and spec-file", []string{"--replicas", "2", "--spec-file", "x"}, "exclusive"},
		{"negative replicas", []string{"--replicas", "-1"}, "cannot be negative"},
		{"group alone", []string{"--group", "default"}, "--group says which container group --replicas applies to"},
		{"wait on a read", []string{"--wait"}, "--wait follows a change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			install(t, seams{})

			_, _, err := run(t, append([]string{id}, tc.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// A change under JSON cannot be asked about, and says which flag answers.
func TestCmd_ChangeUnderJSONNeedsYes(t *testing.T) {
	install(t, seams{get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil }})

	_, _, err := run(t, id, "--replicas", "2", "--output-format", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

// A rollout already in flight refuses the change before anything is sent,
// and before any restart is announced.
func TestCmd_RefusesWhileAReplacementIsInFlight(t *testing.T) {
	install(t, seams{
		get:   func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil },
		guard: func(string) error { return workload.ErrReplacementInFlight },
	})

	_, stderr, err := run(t, id, "--replicas", "2", "--yes")
	require.ErrorIs(t, err, workload.ErrReplacementInFlight)
	assert.NotContains(t, stderr, "rolls the workload's containers")
}

// The body is the whole runtime, so it is built from a read taken after
// the confirmation: a change that landed while the question stood travels
// with the resize instead of being reverted by it.
func TestCmd_ReplicasSendsTheRuntimeAsItIsAfterConfirming(t *testing.T) {
	retuned := strings.Replace(stopped, `"cpu":0.5`, `"cpu":2`, 1)
	reads := 0

	var sent json.RawMessage

	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) {
			reads++

			if reads == 1 {
				return settingsDoc(t, stopped), nil
			}

			return settingsDoc(t, retuned), nil
		},
		update: func(_ string, runtime json.RawMessage) (*workload.Replacement, error) {
			sent = runtime

			return &workload.Replacement{ID: "rep-5", Status: "unknown"}, nil
		},
	})

	_, _, err := run(t, id, "--replicas", "3", "--yes")
	require.NoError(t, err)
	assert.Equal(t, 2, reads, "one read for the refusals, one after the question for the body")
	assert.Contains(t, string(sent), `"cpu":2`, "the later change travels")
	assert.Contains(t, string(sent), `"replicaCount":3`)
}

// --wait follows the replacement, then the workload, then prints the
// settings it is now running on.
func TestCmd_WaitFollowsToTheNewSettings(t *testing.T) {
	scaled := strings.Replace(stopped, `"replicaCount":1`, `"replicaCount":3`, 1)
	reads := 0

	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) {
			reads++

			if reads == 1 {
				return settingsDoc(t, stopped), nil
			}

			return settingsDoc(t, scaled), nil
		},
		update: func(string, json.RawMessage) (*workload.Replacement, error) {
			return &workload.Replacement{ID: "rep-3", Status: "unknown"}, nil
		},
		waitR: func(_ context.Context, _ string, started *workload.Replacement, _, timeout time.Duration, onTick func(*workload.Replacement)) (*workload.Replacement, error) {
			assert.Equal(t, "rep-3", started.ID)
			assert.Equal(t, settleTimeout, timeout)

			onTick(&workload.Replacement{ID: "rep-3", Status: "promoting"})

			return &workload.Replacement{ID: "rep-3", Status: "completed"}, nil
		},
		waitW: func(_ context.Context, _ string, want workload.Serving, _, timeout time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			assert.True(t, want.AwaitDrain, "a resize replaces a generation, so the old one has to stop answering")
			assert.LessOrEqual(t, timeout, settleTimeout, "the second wait gets what the first left, not a fresh budget")
			assert.Positive(t, timeout)

			return &workload.Workload{ID: id, Status: workload.WorkloadStatusRunning}, nil
		},
	})

	stdout, stderr, err := run(t, id, "--replicas", "3", "--yes", "--wait")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Replacement rep-3: promoting")
	assert.Contains(t, stdout, "Settings applied; workload "+id+" is running on them (replacement rep-3)")
	assert.Contains(t, stdout, "3")
	assert.NotContains(t, stdout, "being rolled out", "applied is not in flight")
}

// "applied" is a claim about the count read back, not about the wait having
// returned: a count that is not the one asked for is an error naming both.
func TestCmd_WaitRefusesToCallAMismatchedCountApplied(t *testing.T) {
	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil },
		update: func(string, json.RawMessage) (*workload.Replacement, error) {
			return &workload.Replacement{ID: "rep-6", Status: "unknown"}, nil
		},
		waitR: func(_ context.Context, _ string, started *workload.Replacement, _, _ time.Duration, _ func(*workload.Replacement)) (*workload.Replacement, error) {
			return &workload.Replacement{ID: started.ID, Status: "completed"}, nil
		},
		waitW: func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: id, Status: workload.WorkloadStatusRunning}, nil
		},
	})

	_, _, err := run(t, id, "--replicas", "3", "--yes", "--wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reports 1 replicas, not 3")
}

// A settings body has no single number to read back, so it is applied on
// the replacement's own completed status, and unconfirmed when the wait
// settled without one: the seed comes back when the platform collects a
// fast rollout before the first poll.
func TestCmd_WaitWithoutAVerdictIsUnconfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"containerGroups": [{"name": "default", "replicaCount": 2,
		"resourceBundles": ["cpu.small"], "containers": [{"name": "primary"}]}]}`), 0o600))

	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil },
		update: func(string, json.RawMessage) (*workload.Replacement, error) {
			return &workload.Replacement{ID: "rep-7", Status: "submitted"}, nil
		},
		waitR: func(_ context.Context, _ string, started *workload.Replacement, _, _ time.Duration, _ func(*workload.Replacement)) (*workload.Replacement, error) {
			return started, nil
		},
		waitW: func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: id, Status: workload.WorkloadStatusRunning}, nil
		},
	})

	stdout, _, err := run(t, id, "--spec-file", path, "--yes", "--wait")
	require.NoError(t, err)
	assert.Contains(t, stdout, "no final status for replacement rep-7 (last seen submitted)")
	assert.NotContains(t, stdout, "Settings applied")

	stdout, _, err = run(t, id, "--spec-file", path, "--yes", "--wait", "--output-format", "json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"status": "unconfirmed"`)
}

// A replacement that ends failed leaves the workload on the settings it had,
// and the command says so rather than reporting a bare timeout.
func TestCmd_WaitReportsAFailedReplacement(t *testing.T) {
	install(t, seams{
		get: func(string) (*workload.WorkloadSettings, error) { return settingsDoc(t, stopped), nil },
		update: func(string, json.RawMessage) (*workload.Replacement, error) {
			return &workload.Replacement{ID: "rep-4", Status: "unknown"}, nil
		},
		waitR: func(context.Context, string, *workload.Replacement, time.Duration, time.Duration, func(*workload.Replacement)) (*workload.Replacement, error) {
			return &workload.Replacement{ID: "rep-4", Status: "failed"}, errors.New("replacement rep-4 ended as failed")
		},
	})

	_, _, err := run(t, id, "--replicas", "3", "--yes", "--wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ended as failed")
	assert.Contains(t, err.Error(), "still running with the settings it had")
}
