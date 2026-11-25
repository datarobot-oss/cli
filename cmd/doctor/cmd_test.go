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

package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/features"
	"github.com/datarobot/cli/internal/platform"
)

func stubDescribe(t *testing.T, report *platform.Report, err error) {
	t.Helper()

	prev := describeFn

	describeFn = func(context.Context) (*platform.Report, error) { return report, err }

	t.Cleanup(func() { describeFn = prev })
}

func run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()

	viperx.Reset()
	t.Cleanup(viperx.Reset)

	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	cmd := Cmd()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	err = cmd.Execute()

	return out.String(), err
}

func installReport(workload platform.Section) *platform.Report {
	return &platform.Report{
		Server: platform.Server{Release: "11.15.0"},
		Sections: map[string]platform.Section{
			platform.SectionWorkloadAPI: workload,
		},
	}
}

func answering() platform.Section {
	return platform.Section{Status: platform.StatusOK, Data: platform.WorkloadAPI{}}
}

func refusing(status int) platform.Section {
	return platform.Section{Status: platform.StatusUnavailable, HTTPStatus: status, Message: "refused"}
}

func TestCmd_TextReportHasTheTitleAndEveryCheck(t *testing.T) {
	stubDescribe(t, installReport(answering()), nil)

	stdout, err := run(t)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Platform doctor report")
	assert.Contains(t, stdout, "platform.install.read")
	assert.Contains(t, stdout, "platform.workload.deploy")
	assert.NotContains(t, stdout, "artifact:")
}

func TestCmd_JSONHasTheTitleAndNoProjectFields(t *testing.T) {
	stubDescribe(t, installReport(answering()), nil)

	stdout, err := run(t, "--output-format", "json")
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &decoded), "stdout must be one JSON document")
	assert.Equal(t, "ok", decoded["status"])
	assert.Contains(t, decoded, "title")
	assert.NotContains(t, decoded, "projectDir")
	assert.NotContains(t, decoded, "artifactId")
}

func TestCmd_AWarningExitsZero(t *testing.T) {
	stubDescribe(t, installReport(refusing(404)), nil)

	stdout, err := run(t)
	require.NoError(t, err)

	assert.Contains(t, stdout, "WARN")
}

func TestCmd_AFailureExitsNonZeroAfterPrintingTheReport(t *testing.T) {
	stubDescribe(t, installReport(refusing(502)), nil)

	stdout, err := run(t)

	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Contains(t, stdout, "platform.workload.deploy")
}

func TestCmd_AFailureInJSONStillPrintsOneDocumentAndExitsNonZero(t *testing.T) {
	stubDescribe(t, installReport(refusing(502)), nil)

	stdout, err := run(t, "--output-format", "json")

	require.ErrorIs(t, err, cli.ErrSilent)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &decoded), "stdout must be one JSON document")
	assert.Equal(t, "fail", decoded["status"])
}

func TestCmd_AnUnreadableInstallFailsAndSkipsTheRest(t *testing.T) {
	stubDescribe(t, nil, errors.New("install unreachable"))

	stdout, err := run(t)

	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Contains(t, stdout, "install unreachable")
	assert.Contains(t, stdout, "SKIP")
}

func TestCmd_IsBehindThePlatformDoctorGate(t *testing.T) {
	assert.Equal(t, "platform-doctor", Cmd().Annotations[features.AnnotationKey])
}

func TestUseEnvCredentials_ACompletePairOverridesTheProfile(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	viperx.SetDefault("endpoint", "https://profile.example.com/api/v2")
	viperx.SetDefault("token", "profile-token")
	t.Setenv("DATAROBOT_ENDPOINT", "https://env.example.com/api/v2")
	t.Setenv("DATAROBOT_API_TOKEN", "env-token")

	useEnvCredentials()

	assert.Equal(t, "https://env.example.com/api/v2", viperx.GetString("endpoint"))
	assert.Equal(t, "env-token", viperx.GetString("token"))
}

func TestUseEnvCredentials_AnIncompletePairLeavesTheProfile(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	viperx.SetDefault("endpoint", "https://profile.example.com/api/v2")
	viperx.SetDefault("token", "profile-token")
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "env-token")

	useEnvCredentials()

	assert.Equal(t, "https://profile.example.com/api/v2", viperx.GetString("endpoint"))
	assert.Equal(t, "profile-token", viperx.GetString("token"))
}

func TestCmd_RejectsArguments(t *testing.T) {
	stubDescribe(t, installReport(answering()), nil)

	_, err := run(t, "extra")

	assert.ErrorContains(t, err, `unknown command "extra"`)
}
