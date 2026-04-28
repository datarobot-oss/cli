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

package create

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "spec.json")

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestReadSpecFile_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := readSpecFile(path)
	require.Error(t, err)
	assert.Equal(t, "file not found: "+path, err.Error())
}

func TestReadSpecFile_Valid(t *testing.T) {
	content := `{"name":"x","spec":{"containerGroups":[{"containers":[{}]}]}}`
	path := writeTempFile(t, content)

	got, err := readSpecFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}

func TestCmd_RejectsArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", "x.json", "unexpected-arg"})

	require.Error(t, cmd.Execute())
}

func TestCmd_MissingSpecFile(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `required flag(s) "spec-file" not set`)
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", "x.json", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}
