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

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFormat_Set(t *testing.T) {
	cases := []struct {
		in      string
		want    OutputFormat
		wantErr bool
	}{
		{"text", OutputFormatText, false},
		{"json", OutputFormatJSON, false},
		{"yaml", "", true},
		{"", "", true},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var f OutputFormat

			err := f.Set(c.in)
			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid output format")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.want, f)
		})
	}
}

func TestStatus_Set(t *testing.T) {
	var s Status

	require.NoError(t, s.Set("DRAFT"))
	assert.Equal(t, Status(ArtifactStatusDraft), s)

	require.NoError(t, s.Set("locked"))
	assert.Equal(t, Status(ArtifactStatusLocked), s)

	err := s.Set("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}
