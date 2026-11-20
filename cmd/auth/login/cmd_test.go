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

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunE_SkipAuthReturnsErrSilent covers the --skip-auth short circuit, the one
// login failure path that needs no network or config access. RunE logs the reason
// itself, so it must return cli.ErrSilent; a plain error would have executeRoot print
// the same thing again (CFX-6924).
func TestRunE_SkipAuthReturnsErrSilent(t *testing.T) {
	viperx.Set(config.SkipAuthKey, true)
	t.Cleanup(func() { viperx.Set(config.SkipAuthKey, false) })

	err := RunE(&cobra.Command{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, cli.ErrSilent,
		"the reason is already logged, so the error must not be printed again")
}

// TestCmd_SilenceErrors documents that the command still opts out of cobra's
// printing. Reporting is centralized in executeRoot via cmd.ReportError, which honours
// cli.ErrSilent; this flag keeps cobra from adding a line of its own.
func TestCmd_SilenceErrors(t *testing.T) {
	assert.True(t, Cmd().SilenceErrors)
}

// newLoginCmd returns a login command aimed at a dead endpoint, with no browser
// and the given --timeout.
func newLoginCmd(t *testing.T, timeout string) *cobra.Command {
	t.Helper()

	testutil.SetTestHomeDir(t, t.TempDir())
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.DataRobotURL, "https://nonexistent.invalid")

	cmd := Cmd()
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("no-browser", "true"))
	require.NoError(t, cmd.Flags().Set("timeout", timeout))

	return cmd
}

func TestCmd_HasTimeoutFlag(t *testing.T) {
	f := Cmd().Flags().Lookup("timeout")

	require.NotNil(t, f, "dr auth login must expose --timeout")
	assert.Equal(t, "0s", f.DefValue, "zero default means DefaultLoginTimeout applies")
}

func TestRunE_TimeoutPrintsHelpAndReturnsSilent(t *testing.T) {
	cmd := newLoginCmd(t, "50ms")

	oldStderr := os.Stderr
	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = wErr

	runErr := RunE(cmd, nil)

	require.NoError(t, wErr.Close())

	os.Stderr = oldStderr

	stderr, _ := io.ReadAll(rErr)

	require.ErrorIs(t, runErr, cli.ErrSilent, "a timeout returns the silent sentinel, not the raw error")
	assert.Contains(t, string(stderr), "authorization came back", "the recovery help must reach stderr")
}

func TestRunE_RejectsNegativeTimeout(t *testing.T) {
	err := RunE(newLoginCmd(t, "-1s"), nil)

	assert.ErrorIs(t, err, cli.ErrSilent, "a negative --timeout is rejected before the browser flow starts")
}
