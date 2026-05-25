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
	"io"
	"testing"

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

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "--version", "2", "--output-format", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_RejectsMissingPipeline(t *testing.T) {
	err := runCmd(t, "--version", "2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline")
}

func TestCmd_RejectsZeroVersion(t *testing.T) {
	err := runCmd(t, "--pipeline", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--version")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"pipeline", "version", "offset", "limit", "output-format"} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected --%s flag", name)
	}
}
