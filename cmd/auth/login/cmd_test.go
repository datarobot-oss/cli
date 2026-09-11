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

package login

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_HasTimeoutFlag(t *testing.T) {
	cmd := Cmd()

	f := cmd.Flags().Lookup("timeout")
	require.NotNil(t, f, "dr auth login must expose --timeout")
	assert.Equal(t, "0s", f.DefValue, "zero default means DefaultLoginTimeout applies")

	require.NoError(t, cmd.Flags().Set("timeout", "30s"))

	got, err := cmd.Flags().GetDuration("timeout")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, got)
}

func TestCmd_HasNoBrowserFlag(t *testing.T) {
	assert.NotNil(t, Cmd().Flags().Lookup("no-browser"), "dr auth login must keep --no-browser")
}

// TestRunE_TimeoutPrintsHelpAndReturnsSilent drives the timeout branch end to end:
// a tiny --timeout with no browser and a dead endpoint reaches ErrLoginTimedOut fast.
func TestRunE_TimeoutPrintsHelpAndReturnsSilent(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.DataRobotURL, "https://nonexistent.invalid")

	cmd := Cmd()
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("no-browser", "true"))
	require.NoError(t, cmd.Flags().Set("timeout", "50ms"))

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)

	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout, os.Stderr = wOut, wErr

	runErr := RunE(cmd, nil)

	require.NoError(t, wOut.Close())
	require.NoError(t, wErr.Close())

	os.Stdout, os.Stderr = oldOut, oldErr

	stderr, _ := io.ReadAll(rErr)
	_, _ = io.ReadAll(rOut)

	assert.ErrorIs(t, runErr, cli.ErrSilent, "a timeout returns the silent sentinel, not the raw error")
	assert.Contains(t, string(stderr), "authorization came back", "the recovery help must reach stderr")
}

func TestRunE_RejectsNegativeTimeout(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.DataRobotURL, "https://nonexistent.invalid")

	cmd := Cmd()
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("no-browser", "true"))
	require.NoError(t, cmd.Flags().Set("timeout", "-1s"))

	err := RunE(cmd, nil)
	assert.ErrorIs(t, err, cli.ErrSilent, "a negative --timeout is rejected before the browser flow starts")
}
