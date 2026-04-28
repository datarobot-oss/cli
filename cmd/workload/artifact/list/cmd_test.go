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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--output-format", "xml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "xml"`)
}

func TestCmd_InvalidStatus(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--status", "UNKNOWN"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestCmd_InvalidLimit(t *testing.T) {
	for _, v := range []string{"-1", "0"} {
		cmd := Cmd()
		cmd.PreRunE = nil
		cmd.SetArgs([]string{"--limit", v})

		err := cmd.Execute()
		require.Error(t, err, "limit %s", v)
		assert.Contains(t, err.Error(), "must be positive")
	}
}
