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

package version

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runVersionCmd(t *testing.T, args ...string) string {
	t.Helper()

	cmd := Cmd()

	outBuf := new(bytes.Buffer)

	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(args)

	require.NoError(t, cmd.Execute())

	return outBuf.String()
}

func TestVersionDefaultJSON(t *testing.T) {
	output := runVersionCmd(t)

	var info internalVersion.InfoData

	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &info), "default output should be valid JSON")
	assert.Equal(t, internalVersion.Version, info.Version)
	assert.NotEmpty(t, info.Runtime)
}

func TestVersionOutputFormatText(t *testing.T) {
	output := runVersionCmd(t, "--output-format", "text")

	assert.Contains(t, output, internalVersion.AppName, "text output should contain app name")
	assert.Contains(t, output, internalVersion.Version, "text output should contain version")
}

func TestVersionOutputFormatJSON(t *testing.T) {
	output := runVersionCmd(t, "--output-format", "json")

	var info internalVersion.InfoData

	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &info))
	assert.Equal(t, internalVersion.Version, info.Version)
	assert.NotEmpty(t, info.Runtime)
}

func TestVersionShortText(t *testing.T) {
	output := runVersionCmd(t, "--output-format", "text", "--short")

	assert.Equal(t, internalVersion.Version, strings.TrimSpace(output), "--short should return just the version number")
}

func TestVersionShortJSONIgnored(t *testing.T) {
	output := runVersionCmd(t, "--short")

	// --short is ignored in JSON mode; should still get full JSON
	var info internalVersion.InfoData

	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &info), "output should be valid JSON")
	assert.Equal(t, internalVersion.Version, info.Version)
	assert.NotEmpty(t, info.Runtime)
}

func TestVersionLegacyFormatJSON(t *testing.T) {
	cmd := Cmd()

	outBuf := new(bytes.Buffer)

	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--format", "json"})

	require.NoError(t, cmd.Execute())

	// pflag prints a deprecation warning to the same output buffer; find the JSON line
	var jsonLine string

	for _, line := range strings.Split(outBuf.String(), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "{") {
			jsonLine = line
			break
		}
	}

	require.NotEmpty(t, jsonLine, "should find JSON output among deprecation warnings")

	var info internalVersion.InfoData

	require.NoError(t, json.Unmarshal([]byte(jsonLine), &info), "--format json should produce valid JSON")
	assert.Equal(t, internalVersion.Version, info.Version)
}

func TestVersionLegacyFormatText(t *testing.T) {
	output := runVersionCmd(t, "--format", "text")

	assert.Contains(t, output, internalVersion.AppName, "--format text should produce text output with app name")
	assert.Contains(t, output, internalVersion.Version, "--format text should produce text output with version")
}

func TestVersionInvalidFormat(t *testing.T) {
	cmd := Cmd()

	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--output-format", "yaml"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}
