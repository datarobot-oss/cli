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

package codesync

import (
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetry_AnnotationSet(t *testing.T) {
	cmd := Cmd()
	assert.Contains(t, cmd.Annotations, "telemetry")
}

func TestTelemetry_ExtractorPropertiesAfterFlagParse(t *testing.T) {
	cmd := Cmd()
	require.NoError(t, cmd.ParseFlags([]string{"--dry-run", "--yes", "--output-format=json"}))

	event, ok := telemetry.EventFor(cmd, nil)
	require.True(t, ok, "EventFor must return ok=true for an annotated command")

	assert.Equal(t, true, event.EventProperties["dry_run"])
	assert.Equal(t, false, event.EventProperties["diff"])
	assert.Equal(t, true, event.EventProperties["yes"])
	assert.Equal(t, "json", event.EventProperties["output_format"])
}

// The verify attribute must make --verify runs distinguishable from plain
// runs. It is emitted unconditionally (false without the flag) rather than
// omitted, so a missing key reads as a wiring bug instead of quietly
// blending verify runs into the plain-run population.
func TestTelemetry_VerifyAttribute(t *testing.T) {
	t.Run("false without the flag", func(t *testing.T) {
		cmd := Cmd()
		require.NoError(t, cmd.ParseFlags([]string{"--yes"}))

		event, ok := telemetry.EventFor(cmd, nil)
		require.True(t, ok, "EventFor must return ok=true for an annotated command")

		assert.Contains(t, event.EventProperties, "verify", "the attribute must always be emitted, never omitted")
		assert.Equal(t, false, event.EventProperties["verify"])
	})

	t.Run("true with --verify", func(t *testing.T) {
		cmd := Cmd()
		require.NoError(t, cmd.ParseFlags([]string{"--verify", "--yes"}))

		event, ok := telemetry.EventFor(cmd, nil)
		require.True(t, ok, "EventFor must return ok=true for an annotated command")

		assert.Equal(t, true, event.EventProperties["verify"])
	})
}
