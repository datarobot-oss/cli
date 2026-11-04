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

package tls

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const oneCert = "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"

func TestParseExportOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantPEM   string
		wantErr   bool
	}{
		{
			name:      "count and body",
			raw:       "CERTCOUNT=2\n" + oneCert,
			wantCount: 2,
			wantPEM:   oneCert,
		},
		{
			name:      "empty store reports zero with no body",
			raw:       "CERTCOUNT=0\n",
			wantCount: 0,
			wantPEM:   "",
		},
		{
			name:      "certificates enumerated but nothing encoded",
			raw:       "CERTCOUNT=7\n",
			wantCount: 7,
			wantPEM:   "",
		},
		{
			name:      "CRLF line endings from powershell.exe",
			raw:       "CERTCOUNT=1\r\n" + oneCert + "\r\n",
			wantCount: 1,
			wantPEM:   oneCert,
		},
		{
			name:      "surrounding whitespace is tolerated",
			raw:       "\n  CERTCOUNT=3  \n" + oneCert + "\n\n",
			wantCount: 3,
			wantPEM:   oneCert,
		},
		{
			name:    "missing header is an error, not a silent zero",
			raw:     oneCert,
			wantErr: true,
		},
		{
			name:    "empty output is an error, not an empty store",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "non-numeric count",
			raw:     "CERTCOUNT=lots\n" + oneCert,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pem, count, err := parseExportOutput(tt.raw)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
			assert.Equal(t, tt.wantPEM, pem)
		})
	}
}

// TestParseExportOutputDistinguishesEmptyFromUnencodable is the case that motivated
// the header: an empty store must be told apart from a populated store whose
// certificates could not be encoded, because the advice for each is opposite.
func TestParseExportOutputDistinguishesEmptyFromUnencodable(t *testing.T) {
	t.Parallel()

	_, empty, err := parseExportOutput("CERTCOUNT=0\n")
	require.NoError(t, err)

	pem, populated, err := parseExportOutput("CERTCOUNT=31\n")
	require.NoError(t, err)

	assert.Zero(t, empty, "an empty store must report zero")
	assert.Equal(t, 31, populated, "a populated store must report its count even with no PEM body")
	assert.Empty(t, pem)
}

func TestTruncateForError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `"short output"`, truncateForError("short output"))
	assert.Equal(t, `"a b c"`, truncateForError("a\r\nb\nc"), "newlines collapse to spaces for one-line errors")

	long := truncateForError(strings.Repeat("x", 500))
	assert.Contains(t, long, "…")
	assert.Less(t, len(long), 250, "long output must be truncated")
}
