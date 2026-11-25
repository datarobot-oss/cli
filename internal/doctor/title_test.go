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
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const platformTitle = "Platform doctor report for https://install.example.com"

func titledReport() Report {
	checks := []Result{
		{CheckID: "platform.workload.deploy", Status: StatusWARN, Summary: "no Workload API", Remedy: "use a Custom Application"},
	}

	return NewTitledReport(platformTitle, checks)
}

func TestTextReporter_TitleReplacesProjectHeader(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteText(&buf, titledReport()))

	out := stripANSI(buf.String())

	assert.True(t, strings.HasPrefix(out, platformTitle+"\n"))
	assert.NotContains(t, out, "Doctor report for")
	assert.NotContains(t, out, "artifact:")
	assert.Contains(t, out, "platform.workload.deploy")
}

func TestJSONReporter_TitleReplacesProjectFields(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, titledReport()))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.Equal(t, platformTitle, got["title"])
	assert.NotContains(t, got, "projectDir")
	assert.NotContains(t, got, "artifactId")
	assert.Equal(t, "warn", got["status"])
	assert.Len(t, got["checks"], 1)
}

func TestJSONReporter_NoTitleKeyForProjectReport(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, sampleReport()))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.NotContains(t, got, "title")
	assert.Contains(t, got, "projectDir")
	assert.Contains(t, got, "artifactId")
}
