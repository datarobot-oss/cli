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

package list

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskListJSON(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectFixture(t, projectDir, true)
	writeFakeTaskBinary(t, filepath.Join(t.TempDir(), "task.log"))

	root := &cobra.Command{Use: "test"}

	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	root.AddCommand(Cmd())
	root.SetArgs([]string{"list", "--dir", projectDir, "--output-format", "json"})

	var err error

	out := captureStdout(t, func() { err = root.Execute() })
	require.NoError(t, err)

	var envelope struct {
		Tasks []TaskOutput `json:"tasks"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Len(t, envelope.Tasks, 1)
	assert.Equal(t, "dev", envelope.Tasks[0].Name)
}

func TestTaskListText(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectFixture(t, projectDir, true)
	writeFakeTaskBinary(t, filepath.Join(t.TempDir(), "task.log"))

	root := &cobra.Command{Use: "test"}

	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	root.AddCommand(Cmd())
	root.SetArgs([]string{"list", "--dir", projectDir})

	var err error

	out := captureStdout(t, func() { err = root.Execute() })
	require.NoError(t, err)

	assert.Contains(t, out, "Available Tasks")
	assert.Contains(t, out, "dev")
	assert.NotContains(t, out, `"tasks"`)
}

// captureStdout is local because the shared helper lives under
// cmd/pipeline/internal, which this package cannot import.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	fn()

	require.NoError(t, w.Close())

	os.Stdout = old

	var buf bytes.Buffer

	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

func TestToTaskOutputs(t *testing.T) {
	tasks := []task.Task{
		{
			Name:    "build",
			Aliases: []string{"b", "compile"},
			Desc:    "Build the application\nwith all dependencies",
		},
		{
			Name: "test",
			Desc: "Run tests",
		},
	}

	outputs := toTaskOutputs(tasks)

	require.Len(t, outputs, 2)
	assert.Equal(t, "build", outputs[0].Name)
	assert.Equal(t, []string{"b", "compile"}, outputs[0].Aliases)
	assert.Equal(t, "Build the application with all dependencies", outputs[0].Description)
	assert.Equal(t, "test", outputs[1].Name)
	assert.Empty(t, outputs[1].Aliases)
	assert.Equal(t, "Run tests", outputs[1].Description)
}

func TestToTaskOutputsEmpty(t *testing.T) {
	assert.Empty(t, toTaskOutputs(nil))
	assert.Empty(t, toTaskOutputs([]task.Task{}))
}
