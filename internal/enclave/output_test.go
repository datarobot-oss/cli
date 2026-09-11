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

package enclave

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()

	os.Stdout = old

	var buf bytes.Buffer

	_, _ = io.Copy(&buf, r)

	return buf.String()
}

// The delete result is a contract for scripts: it must distinguish a real
// deletion from the two exit-0 no-ops (already gone, prompt declined).
func TestRenderDeletion_JSON(t *testing.T) {
	cases := map[string]struct {
		result      DeletionResult
		wantDeleted bool
		wantReason  any
	}{
		"deleted": {
			result:      DeletionResult{EnclaveID: "enc-1", Deleted: true},
			wantDeleted: true,
			wantReason:  nil, // omitempty: absent on success
		},
		"not found": {
			result:      DeletionResult{EnclaveID: "enc-1", Reason: DeletionReasonNotFound},
			wantDeleted: false,
			wantReason:  DeletionReasonNotFound,
		},
		"aborted": {
			result:      DeletionResult{EnclaveID: "enc-1", Reason: DeletionReasonAborted},
			wantDeleted: false,
			wantReason:  DeletionReasonAborted,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() {
				require.NoError(t, RenderDeletion(outputformat.OutputFormatJSON, tc.result))
			})

			var back map[string]any

			require.NoError(t, json.Unmarshal([]byte(out), &back))
			assert.Equal(t, "enc-1", back["enclaveId"])
			assert.Equal(t, tc.wantDeleted, back["deleted"])
			assert.Equal(t, tc.wantReason, back["reason"])
		})
	}
}

func TestRenderDeletion_Text(t *testing.T) {
	cases := map[string]struct {
		result DeletionResult
		want   string
	}{
		"deleted":   {DeletionResult{EnclaveID: "enc-1", Deleted: true}, "Deleted enclave: enc-1"},
		"not found": {DeletionResult{EnclaveID: "enc-1", Reason: DeletionReasonNotFound}, "No enclave found with id: enc-1"},
		"aborted":   {DeletionResult{EnclaveID: "enc-1", Reason: DeletionReasonAborted}, "Aborted."},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() {
				require.NoError(t, RenderDeletion(outputformat.OutputFormatText, tc.result))
			})

			assert.Contains(t, out, tc.want)
		})
	}
}

func TestPosixQuote(t *testing.T) {
	assert.Equal(t, `'plain'`, posixQuote("plain"))
	assert.Equal(t, `'a b'`, posixQuote("a b"))
	assert.Equal(t, `'it'\''s'`, posixQuote("it's"))
	assert.Equal(t, `''`, posixQuote(""))
}

// Secret data is server-supplied and lands in a command the operator pastes
// into a shell, so every key=value must survive as exactly one literal word.
func TestKubectlLiterals_QuotesServerData(t *testing.T) {
	got := kubectlLiterals(map[string]string{
		"token": "$(whoami)",
		"cert":  "-----BEGIN CERT-----\nline two\n",
	})

	// Sorted by key, each key=value wrapped as a single quoted word.
	assert.Equal(t,
		"--from-literal='cert=-----BEGIN CERT-----\nline two\n' "+
			"--from-literal='token=$(whoami)'",
		got)

	// The substitution is inert because the whole word is single-quoted; a
	// bare $(...) here would run on paste.
	assert.Contains(t, got, `'token=$(whoami)'`)
}

func TestKubectlLiterals_EscapesEmbeddedQuote(t *testing.T) {
	got := kubectlLiterals(map[string]string{"k": "it's"})
	assert.Equal(t, `--from-literal='k=it'\''s'`, got)
}
