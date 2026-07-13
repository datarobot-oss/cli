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

package result

import (
	"bytes"
	"io"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCmd(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	return cmd.Execute()
}

func TestCmd_RejectsMissingPipeline(t *testing.T) {
	err := runCmd(t, "--run", "d-1", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline")
}

func TestCmd_RejectsMissingRun(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run")
}

func TestCmd_RequiresPositionalArg(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1")
	require.Error(t, err)
}

func TestCmd_RejectsNonIntegerTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1", "notanumber")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid task-id")
}

func TestCmd_RejectsZeroTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1", "0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid task-id")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"pipeline", "run", "output-format"} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected --%s flag", name)
	}
}

func TestPrintResultHuman_TextPreview(t *testing.T) {
	notSerializable := "not_json_serializable"
	noResult := "no_result_recorded"

	tests := []struct {
		name        string
		res         pipeline.TaskExecutionResult
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "unavailable value with truncated text preview",
			res: pipeline.TaskExecutionResult{
				URL:                    "https://s3.example.com/result.tobj",
				ExpiresIn:              900,
				ContentType:            "application/octet-stream",
				ValueAvailable:         false,
				ValueUnavailableReason: &notSerializable,
				ValueText:              "   x  y\n0  1  3\n1  2  4",
				ValueTextTruncated:     true,
			},
			wantContain: []string{
				"(not available: not_json_serializable)",
				"Text Preview (truncated):",
				"0  1  3",
			},
			wantAbsent: []string{},
		},
		{
			name: "unavailable value with non-truncated text preview",
			res: pipeline.TaskExecutionResult{
				URL:                    "https://s3.example.com/result.tobj",
				ExpiresIn:              900,
				ContentType:            "application/octet-stream",
				ValueAvailable:         false,
				ValueUnavailableReason: &notSerializable,
				ValueText:              "some repr",
				ValueTextTruncated:     false,
			},
			wantContain: []string{"Text Preview:", "some repr"},
			wantAbsent:  []string{"(truncated)"},
		},
		{
			name: "unavailable value with no text preview",
			res: pipeline.TaskExecutionResult{
				URL:                    "https://s3.example.com/result.tobj",
				ExpiresIn:              900,
				ContentType:            "application/octet-stream",
				ValueAvailable:         false,
				ValueUnavailableReason: &noResult,
				ValueText:              "",
			},
			wantContain: []string{"(not available: no_result_recorded)"},
			wantAbsent:  []string{"Text Preview"},
		},
		{
			name: "available value suppresses text preview",
			res: pipeline.TaskExecutionResult{
				URL:            "https://s3.example.com/result.tobj",
				ExpiresIn:      900,
				ContentType:    "application/octet-stream",
				Value:          "42",
				ValueAvailable: true,
				// A text preview may still be present, but the JSON value
				// wins and the text preview block must be suppressed.
				ValueText: "should not be shown",
			},
			wantContain: []string{"Value Preview:", "42"},
			wantAbsent:  []string{"Text Preview", "should not be shown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.res

			var buf bytes.Buffer

			printResultHuman(&buf, &res)

			out := buf.String()

			for _, want := range tt.wantContain {
				assert.Contains(t, out, want)
			}

			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, out, absent)
			}
		})
	}
}

// TestRenderResult_JSONIncludesTruncatedFlag guards the valueTextTruncated
// JSON tag: it must NOT use omitempty, so a false value stays in the output
// and "not truncated" is distinguishable from "field absent".
func TestRenderResult_JSONIncludesTruncatedFlag(t *testing.T) {
	res := &pipeline.TaskExecutionResult{
		URL:                "https://s3.example.com/result.tobj",
		ValueAvailable:     false,
		ValueText:          "some repr",
		ValueTextTruncated: false,
	}

	var buf bytes.Buffer

	require.NoError(t, renderResult(&buf, outputformat.OutputFormatJSON, res))

	out := buf.String()

	assert.Contains(t, out, `"valueText": "some repr"`)
	assert.Contains(t, out, `"valueTextTruncated": false`)
}
