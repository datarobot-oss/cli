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
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rejected URL arg must abort, not fall through to login against the
// previously configured endpoint.
func TestRunE_RejectsInvalidURLArg(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	viperx.Reset()
	t.Cleanup(viperx.Reset)

	const existing = "https://app.eu.datarobot.com/api/v2"

	viperx.Set(config.DataRobotURL, existing)

	cmd := Cmd()
	cmd.SetContext(context.Background())

	err := RunE(cmd, []string{"ftp://app.datarobot.com"})

	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Equal(t, existing, viperx.GetString(config.DataRobotURL), "the configured endpoint must be untouched")
}
