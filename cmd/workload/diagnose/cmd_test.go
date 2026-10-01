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

package diagnose

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubDiagnose(t *testing.T, d *workload.Diagnosis, err error) *string {
	t.Helper()

	asked := new(string)
	prev := diagnoseFn

	diagnoseFn = func(id string) (*workload.Diagnosis, error) {
		*asked = id

		return d, err
	}

	t.Cleanup(func() { diagnoseFn = prev })

	return asked
}

func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := Cmd()

	var out, errOut bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	// The command's own PreRunE authenticates, which a unit test must not do.
	cmd.PreRunE = nil

	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func TestCmd_ArgIsOptional(t *testing.T) {
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	stubDiagnose(t, &workload.Diagnosis{}, nil)

	_, _, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

// An errored workload exits zero: the command exists to explain that state.
func TestCmd_ErroredWorkloadIsNotAFailure(t *testing.T) {
	asked := stubDiagnose(t, &workload.Diagnosis{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Name: "app", Status: workload.WorkloadStatusErrored,
		Generations: []workload.GenerationDiagnosis{{
			ID: "p1", ArtifactID: "art-1", Status: "errored", Role: "active",
			Details:  &workload.ProtonStatusDetails{},
			Findings: []string{"primary: CrashLoopBackOff; last run exited 1"},
		}},
	}, nil)

	stdout, _, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0")
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", *asked)
	assert.Contains(t, stdout, "errored")
	assert.Contains(t, stdout, "⚠ primary: CrashLoopBackOff; last run exited 1")
}

// Under JSON, stdout is one document and nothing else.
func TestCmd_JSONIsTheWholeOfStdout(t *testing.T) {
	stubDiagnose(t, &workload.Diagnosis{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: workload.WorkloadStatusErrored,
		Generations: []workload.GenerationDiagnosis{{ID: "p1", Findings: []string{}}},
	}, nil)

	stdout, _, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout must be one JSON document")

	body, ok := envelope["diagnosis"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "errored", body["status"])
}

// A read that fails is the command's failure, carrying the client's message.
// (With a typed id and a plain error, ref.Wrap passes the error through; the
// manifest-sourced wording is idargs' own and tested there.)
func TestCmd_ReadFailureIsAnError(t *testing.T) {
	stubDiagnose(t, nil, errors.New("proton status details: HTTP 403"))

	_, _, err := run(t, "68b0c1d2e3f4a5b6c7d8e9f0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
}

func TestCmd_HasNoWaitFlag(t *testing.T) {
	cmd := Cmd()

	assert.Nil(t, cmd.Flag("wait"))
	assert.Nil(t, cmd.Flag("poll-interval"))
}
