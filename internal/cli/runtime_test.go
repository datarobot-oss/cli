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

package cli

import (
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNonInteractiveEnv(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "1")

	assert.True(t, IsNonInteractive(nil))
}

func TestIsNonInteractiveFlag(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().BoolP(YesFlagName, "y", false, "skip prompts")
	require.NoError(t, cmd.ParseFlags([]string{"--" + YesFlagName}))

	assert.True(t, IsNonInteractive(cmd))
}

func TestIsNonInteractiveForceInteractiveOverrides(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "true")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool(YesFlagName, false, "skip prompts")
	require.NoError(t, cmd.ParseFlags([]string{"--" + YesFlagName}))

	viperx.Set("force-interactive", true)

	assert.False(t, IsNonInteractive(cmd))
}
