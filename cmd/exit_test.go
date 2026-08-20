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

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/stretchr/testify/assert"
)

func TestReportError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error prints nothing",
			err:  nil,
			want: "",
		},
		{
			name: "ErrSilent prints nothing because the command already reported it",
			err:  cli.ErrSilent,
			want: "",
		},
		{
			name: "an error wrapping ErrSilent is still silent",
			err:  fmt.Errorf("auth check: %w", cli.ErrSilent),
			want: "",
		},
		{
			name: "an ordinary error is reported",
			err:  errors.New("boom"),
			want: "Error: boom\n",
		},
		{
			name: "a TLS setup failure from the root PersistentPreRunE is reported",
			err:  errors.New(`apply tls options: reading CA cert "C:\nope.pem": no such file or directory`),
			want: "Error: apply tls options: reading CA cert \"C:\\nope.pem\": no such file or directory\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			ReportError(&buf, tt.err)

			assert.Equal(t, tt.want, buf.String())
		})
	}
}

// TestRootSilencesErrorsForSingleReporting locks in the contract that makes
// ReportError the only error reporter. If cobra were allowed to print too, errors
// raised by the root PersistentPreRunE would either double-print or, for commands
// setting SilenceErrors: true, disappear entirely (CFX-6924).
func TestRootSilencesErrorsForSingleReporting(t *testing.T) {
	t.Parallel()

	assert.True(t, RootCmd.SilenceErrors,
		"RootCmd.SilenceErrors must stay true so cmd.ReportError is the single error reporter")
}
