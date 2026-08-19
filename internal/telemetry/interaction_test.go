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

package telemetry

import (
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStampInteractionMode_NonInteractiveEnv(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "1")

	props := &CommonProperties{}

	StampInteractionMode(props, nil)

	// The environment variable alone should flip the telemetry flag.
	assert.True(t, props.NonInteractive)
}

func TestStampInteractionMode_YesFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().BoolP("yes", "y", false, "skip prompts")

	require.NoError(t, cmd.ParseFlags([]string{"--yes"}))

	props := &CommonProperties{}

	StampInteractionMode(props, cmd)

	// Any explicit --yes invocation is tracked as non-interactive.
	assert.True(t, props.NonInteractive)
}

func TestStampInteractionMode_ForceInteractiveOverrides(t *testing.T) {
	defer viperx.Reset()

	// Simulate both automation signals; force-interactive should still win.
	t.Setenv(reader.NonInteractiveEnv, "true")
	viperx.Set("force-interactive", true)

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("yes", false, "skip prompts")
	require.NoError(t, cmd.ParseFlags([]string{"--yes"}))

	props := &CommonProperties{NonInteractive: true}

	StampInteractionMode(props, cmd)

	assert.False(t, props.NonInteractive)
}
