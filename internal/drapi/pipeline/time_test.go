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

package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantUTC string
		wantErr bool
	}{
		{
			name:    "RFC3339 with Z",
			input:   `"2026-05-20T15:39:57Z"`,
			wantUTC: "2026-05-20T15:39:57Z",
		},
		{
			name:    "RFC3339Nano with Z",
			input:   `"2026-05-20T15:39:57.913317Z"`,
			wantUTC: "2026-05-20T15:39:57.913317Z",
		},
		{
			name:    "naive UTC datetime (no timezone indicator)",
			input:   `"2026-05-20T15:39:57.913317"`,
			wantUTC: "2026-05-20T15:39:57.913317Z",
		},
		{
			name:    "RFC3339 with positive offset",
			input:   `"2026-05-20T17:39:57+02:00"`,
			wantUTC: "2026-05-20T15:39:57Z",
		},
		{
			name:    "empty string yields zero time",
			input:   `""`,
			wantUTC: "0001-01-01T00:00:00Z",
		},
		{
			name:    "null yields zero time",
			input:   `"null"`,
			wantUTC: "0001-01-01T00:00:00Z",
		},
		{
			name:    "invalid format returns error",
			input:   `"not-a-date"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time

			err := json.Unmarshal([]byte(tt.input), &got)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUTC, got.UTC().Format(time.RFC3339Nano))
		})
	}
}

func TestTime_UnmarshalJSON_ViaStruct(t *testing.T) {
	type payload struct {
		CreatedAt Time `json:"created_at"`
	}

	raw := `{"created_at":"2026-04-28T11:42:28.000000"}`

	var p payload

	require.NoError(t, json.Unmarshal([]byte(raw), &p))
	assert.Equal(t, "2026-04-28T11:42:28Z", p.CreatedAt.UTC().Format(time.RFC3339))
}
