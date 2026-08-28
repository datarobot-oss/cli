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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ansiPattern matches ANSI SGR escape sequences so tests can assert on plain text.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// sampleReport returns a report exercising every status plus details and actions.
func sampleReport() Report {
	artifact := "abc123"

	remedy := "dr artifact code doctor --relink <new-artifact-id> & sync"

	checks := []Result{
		{CheckID: "wapi.presence", Status: StatusOK, Summary: "linked"},
		{CheckID: "wapi.config", Status: StatusOK, Summary: "valid"},
		{CheckID: "wapi.manifest", Status: StatusWARN, Summary: "rebuildable", Remedy: "dr artifact code doctor --fix", Fixable: true},
		{CheckID: "wapi.divergence", Status: StatusFAIL, Summary: "diverged", Remedy: remedy, Details: map[string]string{"path": "/tmp/x/manifest.json"}, Fixable: true},
		{CheckID: "wapi.lock", Status: StatusSKIP, Summary: "not enforced"},
	}

	return NewReport("/tmp/x", &artifact, checks)
}

func TestTextReporter_HeaderTableRemediesSummary(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteText(&buf, sampleReport()))

	out := stripANSI(buf.String())

	// Header: absolute project dir + linked artifact id.
	assert.Contains(t, out, "/tmp/x")
	assert.Contains(t, out, "abc123")

	// Table headers and one row per check (in runner order).
	for _, want := range []string{"CHECK", "STATUS", "DETAIL", "wapi.presence", "wapi.config", "wapi.manifest", "wapi.divergence", "wapi.lock", "OK", "WARN", "FAIL", "SKIP"} {
		assert.Contains(t, out, want)
	}

	// Order check: presence row appears before divergence row.
	assert.Less(t, strings.Index(out, "wapi.presence"), strings.Index(out, "wapi.divergence"))

	// Remedies rendered for non-OK rows.
	assert.Contains(t, out, "dr artifact code doctor --fix")
	assert.Contains(t, out, "dr artifact code doctor --relink <new-artifact-id> & sync")

	// Summary line: counts plus verdict.
	assert.Contains(t, out, "2 ok")
	assert.Contains(t, out, "1 warn")
	assert.Contains(t, out, "1 fail")
	assert.Contains(t, out, "1 skip")
	assert.Contains(t, out, "verdict: fail")
}

func TestTextReporter_NotLinked(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteText(&buf, NewReport("/tmp/x", nil, nil)))

	out := stripANSI(buf.String())

	assert.Contains(t, out, "not linked")
	assert.NotContains(t, out, "abc123")
}

func TestTextReporter_NoRemediesWhenAllOK(t *testing.T) {
	var buf bytes.Buffer

	report := NewReport("/tmp/x", nil, []Result{{CheckID: "a.b", Status: StatusOK, Summary: "fine"}})

	require.NoError(t, WriteText(&buf, report))

	out := stripANSI(buf.String())

	assert.Contains(t, out, "verdict: ok")
	assert.NotContains(t, out, "Remedies")
}

func TestTextReporter_DetailsInRow(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteText(&buf, sampleReport()))

	out := stripANSI(buf.String())

	// details.path surfaces in the human-readable DETAIL column.
	assert.Contains(t, out, "path: /tmp/x/manifest.json")
}

func TestJSONReporter_Schema(t *testing.T) {
	var buf bytes.Buffer

	report := sampleReport()

	report.Actions = &[]Action{{ID: "wapi.manifest", Status: ActionSkipped, Reason: "sync in progress"}}

	require.NoError(t, WriteJSON(&buf, report))

	// Raw-bytes assertion (BEFORE json.Unmarshal): SetEscapeHTML(false) must
	// leave <, >, and & verbatim in the marshaled output. The post-Unmarshal
	// assertions below are escaping-invariant (Decode reverses HTML escaping),
	// so only this check pins the encoder configuration documented in json.go.
	raw := buf.String()

	assert.Contains(t, raw, "<new-artifact-id>")
	assert.Contains(t, raw, "& sync")
	assert.NotContains(t, raw, `\u003c`)
	assert.NotContains(t, raw, `\u003e`)
	assert.NotContains(t, raw, `\u0026`)

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.Equal(t, "/tmp/x", got["projectDir"])
	assert.Equal(t, "abc123", got["artifactId"])
	assert.Equal(t, "fail", got["status"])

	summary, ok := got["summary"].(map[string]any)

	require.True(t, ok)

	// JSON numbers decode as float64; InDelta keeps testifylint's
	// float-compare rule happy.
	assert.InDelta(t, 2, summary["ok"], 0)
	assert.InDelta(t, 1, summary["warn"], 0)
	assert.InDelta(t, 1, summary["fail"], 0)
	assert.InDelta(t, 1, summary["skip"], 0)

	// Checks array in runner order, uppercase per-check status.
	rawChecks, ok := got["checks"].([]any)

	require.True(t, ok)

	require.Len(t, rawChecks, 5)

	wantIDs := []string{"wapi.presence", "wapi.config", "wapi.manifest", "wapi.divergence", "wapi.lock"}

	for i, raw := range rawChecks {
		check, ok := raw.(map[string]any)

		require.True(t, ok)

		assert.Equal(t, wantIDs[i], check["id"])
	}

	diverged := rawChecks[3].(map[string]any)

	assert.Equal(t, "FAIL", diverged["status"])
	assert.Equal(t, "diverged", diverged["summary"])
	assert.Equal(t, "dr artifact code doctor --relink <new-artifact-id> & sync", diverged["remedy"])
	assert.Equal(t, true, diverged["fixable"])

	details, ok := diverged["details"].(map[string]any)

	require.True(t, ok)

	assert.Equal(t, "/tmp/x/manifest.json", details["path"])

	// Actions present for repair runs.
	actions, ok := got["actions"].([]any)

	require.True(t, ok)

	require.Len(t, actions, 1)

	action := actions[0].(map[string]any)

	assert.Equal(t, "wapi.manifest", action["id"])
	assert.Equal(t, "skipped", action["status"])
	assert.Equal(t, "sync in progress", action["reason"])
}

func TestJSONReporter_PureSingleJSONObject(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, sampleReport()))

	out := buf.String()

	assert.True(t, json.Valid([]byte(out)))
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(out), "}"))
}

// TestJSONReporter_SingleObjectSecondDecodeEOF pins "exactly one JSON object":
// a second decode of the same stream must hit EOF.
func TestJSONReporter_SingleObjectSecondDecodeEOF(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, sampleReport()))

	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))

	var obj map[string]any

	require.NoError(t, dec.Decode(&obj))

	var extra map[string]any

	err := dec.Decode(&extra)

	require.Error(t, err)

	assert.Equal(t, "EOF", err.Error())
}

func TestJSONReporter_ArtifactIDNullWhenUnlinked(t *testing.T) {
	var buf bytes.Buffer

	report := NewReport("/tmp/x", nil, []Result{{CheckID: "a.b", Status: StatusOK, Summary: "fine"}})

	require.NoError(t, WriteJSON(&buf, report))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	val, present := got["artifactId"]

	assert.True(t, present, "artifactId key must be present, not omitted")
	assert.Nil(t, val)
}

func TestJSONReporter_EmptyStringArtifactIDNormalizedToNull(t *testing.T) {
	// Empty ≈ nil normalization (pinned): never emit "".
	empty := ""

	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, NewReport("/tmp/x", &empty, nil)))

	assert.Contains(t, buf.String(), "\"artifactId\": null")
	assert.NotContains(t, buf.String(), "\"artifactId\": \"\"")
}

func TestJSONReporter_ActionsOmittedForReadOnly(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, sampleReport()))

	assert.NotContains(t, buf.String(), "actions")
}

func TestJSONReporter_ActionsEmptyArrayForRepairRun(t *testing.T) {
	// A repair run with nothing to do still emits an (empty) actions array.
	var buf bytes.Buffer

	report := sampleReport()

	report.Actions = &[]Action{}

	require.NoError(t, WriteJSON(&buf, report))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	actions, present := got["actions"]

	require.True(t, present, "actions key must be present for repair runs")

	assert.Empty(t, actions)
}

func TestJSONReporter_StatusCasing(t *testing.T) {
	// Top-level status lowercase, per-check status uppercase (pinned).
	var buf bytes.Buffer

	report := NewReport("/tmp/x", nil, []Result{{CheckID: "a.b", Status: StatusWARN, Summary: "meh"}})

	require.NoError(t, WriteJSON(&buf, report))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.Equal(t, "warn", got["status"])

	checks := got["checks"].([]any)

	assert.Equal(t, "WARN", checks[0].(map[string]any)["status"])
}

func TestJSONReporter_NoDetailsKeyWhenNil(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteJSON(&buf, sampleReport()))

	var got map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	checks := got["checks"].([]any)

	presence := checks[0].(map[string]any)

	_, hasDetails := presence["details"]

	assert.False(t, hasDetails, "details must be omitted when nil")
}
