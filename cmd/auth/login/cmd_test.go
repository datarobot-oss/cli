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
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunE_SkipAuthReturnsErrSilent covers the --skip-auth short circuit, the one
// login failure path that needs no network or config access. RunE logs the reason
// itself, so it must return cli.ErrSilent; a plain error would have main.go print
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
// printing. Reporting is centralized in main.go via cmd.ReportError, which honours
// cli.ErrSilent; this flag keeps cobra from adding a line of its own.
func TestCmd_SilenceErrors(t *testing.T) {
	assert.True(t, Cmd().SilenceErrors)
}
