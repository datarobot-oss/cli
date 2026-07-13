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

package logs

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

func TestCmd_RequiresImageFlag(t *testing.T) {
	err := runCmd(t, "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestCmd_RequiresVersionArg(t *testing.T) {
	err := runCmd(t, "--image", "img-1")
	require.Error(t, err)
}

func TestCmd_RejectsNonIntegerVersion(t *testing.T) {
	err := runCmd(t, "--image", "img-1", "notanumber")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestCmd_RejectsZeroVersion(t *testing.T) {
	err := runCmd(t, "--image", "img-1", "0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()
	assert.NotNil(t, cmd.Flags().Lookup("image"), "expected --image flag")
}
