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

package display

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countDocuments reports how many top-level JSON documents b contains. The
// whole point of RAPTOR-19348 is that this is always 1.
func countDocuments(t *testing.T, b []byte) int {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(b))

	n := 0

	for {
		var doc json.RawMessage

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		n++
	}

	return n
}

// An executed sync renders one document: the plan at the top level with the
// result nested under "result".
func TestRenderSyncJSON_OneDocumentWithNestedResult(t *testing.T) {
	plan := &sync.SyncPlan{
		Uploads:         []sync.FileAction{{Path: "a.py"}},
		OldVersionShort: "abcd1234",
	}
	result := &sync.Result{NewVersion: "v2", UploadedCount: 1, Duration: 3 * time.Second}

	var buf bytes.Buffer

	require.NoError(t, RenderSyncJSON(&buf, plan, result, false))

	assert.Equal(t, 1, countDocuments(t, buf.Bytes()), "must be exactly one JSON document")

	var doc struct {
		Uploads []FileActionJSON `json:"uploads"`
		Locked  bool             `json:"locked"`
		Result  *ResultJSON      `json:"result"`
	}

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc), "the whole stream is one object")
	require.Len(t, doc.Uploads, 1)
	assert.Equal(t, "a.py", doc.Uploads[0].Path)
	require.NotNil(t, doc.Result)
	assert.Equal(t, "v2", doc.Result.NewVersion)
}

// A preview (no execute) renders the same single document with no "result"
// key, so a plan-only run stays parseable in one Unmarshal too.
func TestRenderSyncJSON_OmitsResultWhenNil(t *testing.T) {
	plan := &sync.SyncPlan{Downloads: []sync.FileAction{{Path: "b.py"}}}

	var buf bytes.Buffer

	require.NoError(t, RenderSyncJSON(&buf, plan, nil, true))

	assert.Equal(t, 1, countDocuments(t, buf.Bytes()))
	assert.NotContains(t, buf.String(), `"result"`, "a plan-only run carries no result key")

	var doc struct {
		Downloads []FileActionJSON `json:"downloads"`
		Locked    bool             `json:"locked"`
	}

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Downloads, 1)
	assert.True(t, doc.Locked, "the locked flag rides the top level, as before")
}
