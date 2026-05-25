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

package update

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUpdateBody_RequiresAtLeastOneField(t *testing.T) {
	cmd := Cmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.ParseFlags([]string{"--pipeline=p", "--version=2"}))

	_, err := buildUpdateBody(cmd, "p", 2, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of --cron")
}

func TestBuildUpdateBody_PicksUpChangedFlags(t *testing.T) {
	cmd := Cmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.ParseFlags([]string{
		"--pipeline=p", "--version=2",
		"--cron=*/5 * * * *",
		"--timezone=America/Los_Angeles",
	}))

	body, err := buildUpdateBody(cmd, "p", 2, "*/5 * * * *", "America/Los_Angeles")
	require.NoError(t, err)
	require.NotNil(t, body.CronExpression)
	require.NotNil(t, body.Timezone)
	assert.Equal(t, "*/5 * * * *", *body.CronExpression)
	assert.Equal(t, "America/Los_Angeles", *body.Timezone)
}

func TestBuildUpdateBody_SkipsUnchangedFlags(t *testing.T) {
	cmd := Cmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	// only --cron supplied; --timezone untouched
	require.NoError(t, cmd.ParseFlags([]string{
		"--pipeline=p", "--version=2",
		"--cron=0 0 * * *",
	}))

	body, err := buildUpdateBody(cmd, "p", 2, "0 0 * * *", "")
	require.NoError(t, err)
	require.NotNil(t, body.CronExpression)
	assert.Equal(t, "0 0 * * *", *body.CronExpression)
	assert.Nil(t, body.Timezone, "untouched --timezone should not be sent")
}

func TestBuildUpdateBody_RejectsZeroVersion(t *testing.T) {
	cmd := Cmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.ParseFlags([]string{"--pipeline=p", "--cron=0 0 * * *"}))

	_, err := buildUpdateBody(cmd, "p", 0, "0 0 * * *", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--version")
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"sched-id", "--pipeline=p", "--version=2", "--cron=0 0 * * *", "--output-format=yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}
