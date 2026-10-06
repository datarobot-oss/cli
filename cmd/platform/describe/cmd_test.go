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

package describe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubDescribe(t *testing.T, report *platform.Report, err error) {
	t.Helper()

	prev := describeFn

	describeFn = func(context.Context) (*platform.Report, error) { return report, err }

	t.Cleanup(func() { describeFn = prev })
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

func sampleReport() *platform.Report {
	return &platform.Report{
		SchemaVersion: platform.SchemaVersion,
		Producer:      platform.Producer{Name: "dr", Version: "test"},
		Server:        platform.Server{Release: "11.12.0", APIVersion: "2.48", CanonicalURL: "https://dr.example.com"},
		Sections: map[string]platform.Section{
			platform.SectionSeats: {Status: platform.StatusOK, Data: platform.Seats{SeatLicenses: map[string]bool{}}},
		},
	}
}

func TestCmd_JSONPrintsTheReportAndNothingElse(t *testing.T) {
	stubDescribe(t, sampleReport(), nil)

	stdout, _, err := run(t, "--output-format", "json")
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &decoded), "stdout must be one JSON document")
	assert.EqualValues(t, 1, decoded["schemaVersion"])
}

func TestCmd_TextIsTheDefault(t *testing.T) {
	stubDescribe(t, sampleReport(), nil)

	stdout, _, err := run(t)
	require.NoError(t, err)

	assert.Contains(t, stdout, "https://dr.example.com")
	assert.Contains(t, stdout, "seats: ok")
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	stubDescribe(t, sampleReport(), nil)

	_, _, err := run(t, "--output-format", "yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func TestCmd_ADescribeErrorFailsTheCommandAndPrintsNoReport(t *testing.T) {
	stubDescribe(t, nil, errors.New("cannot reach the install: no route"))

	stdout, _, err := run(t, "--output-format", "json")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach the install")
	assert.Empty(t, stdout)
}

func TestCmd_TakesNoArguments(t *testing.T) {
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"extra"}))
}
