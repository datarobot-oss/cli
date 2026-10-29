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

package jobargs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFields = []Field{{Flag: "name", Key: "name"}, {Flag: "artifact-id", Key: "artifactId"}}

func withFlags(t *testing.T, args ...string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("artifact-id", "", "")
	require.NoError(t, cmd.Flags().Parse(args))

	return cmd
}

func specFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "spec.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestBody_FlagsOnly(t *testing.T) {
	body, err := Body(withFlags(t, "--name", "train"), "", testFields)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "train"}, body)
}

func TestBody_AFlagFillsWhatTheSpecLeavesOut(t *testing.T) {
	path := specFile(t, "name: train\nfutureField: 3\n")

	body, err := Body(withFlags(t, "--artifact-id", "art-1"), path, testFields)

	require.NoError(t, err)
	assert.Equal(t, "train", body["name"])
	assert.Equal(t, "art-1", body["artifactId"])
	// Fields the CLI does not know reach the server.
	assert.InDelta(t, 3, body["futureField"], 0)
}

func TestBody_AFlagMayNotContradictTheSpec(t *testing.T) {
	path := specFile(t, "name: train\n")

	_, err := Body(withFlags(t, "--name", "other"), path, testFields)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "contradicts name")
}

func TestBody_AFlagThatAgreesWithTheSpecIsFine(t *testing.T) {
	path := specFile(t, "name: train\n")

	_, err := Body(withFlags(t, "--name", "train"), path, testFields)

	require.NoError(t, err)
}

func TestBody_TheSpecMustBeAnObject(t *testing.T) {
	path := specFile(t, "- a\n- b\n")

	_, err := Body(withFlags(t), path, testFields)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must hold a JSON object")
}

func TestRuntimeFromFile(t *testing.T) {
	path := specFile(t, "containerGroups:\n  - name: default\n    resourceBundles: [gpu.a100.1]\n")

	runtime, err := RuntimeFromFile(path)

	require.NoError(t, err)
	assert.Contains(t, runtime, "containerGroups")
}

func confirmCmd(t *testing.T, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	var format outputformat.OutputFormat

	cmd := &cobra.Command{Use: "delete"}
	AddYesFlag(cmd, "")
	outputformat.AddFlag(cmd, &format)
	require.NoError(t, cmd.Flags().Parse(args))

	var errOut bytes.Buffer

	cmd.SetErr(&errOut)

	return cmd, &errOut
}

func TestConfirm_YesAnswersIt(t *testing.T) {
	cmd, _ := confirmCmd(t, "--yes")

	confirmed, err := Confirm(cmd, "go? ")

	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirm_ADeclaredNonInteractiveRunAnswersIt(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "1")
	t.Cleanup(viperx.Reset)

	cmd, _ := confirmCmd(t)

	confirmed, err := Confirm(cmd, "go? ")

	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirm_JSONOutputCannotAsk(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd, errOut := confirmCmd(t, "--output-format", "json")

	_, err := Confirm(cmd, "go? ")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --yes")
	assert.Empty(t, errOut.String())
}
