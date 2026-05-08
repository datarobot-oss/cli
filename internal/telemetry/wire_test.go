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

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestTrack_AddsAnnotationAndProducesEvent(t *testing.T) {
	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "ping"}

	root.AddCommand(cmd)

	Track(cmd)

	assert.Contains(t, cmd.Annotations, trackAnnotation, "Track should set the telemetry annotation")

	event, ok := EventFor(cmd, []string{"ignored"})
	assert.True(t, ok)
	assert.Equal(t, "dr ping", event.EventType)
	assert.Empty(t, event.EventProperties)
	assert.False(t, IsPluginCommand(cmd))
}

func TestTrackWith_PassesPropertiesFromExtractor(t *testing.T) {
	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "run"}

	root.AddCommand(cmd)

	TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"task_name": FirstArg(args),
		}
	})

	event, ok := EventFor(cmd, []string{"build"})
	assert.True(t, ok)
	assert.Equal(t, "dr run", event.EventType)
	assert.Equal(t, "build", event.EventProperties["task_name"])
	assert.False(t, IsPluginCommand(cmd))
}

func TestTrackPlugin_AddsPluginVersionAndPluginAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "assist"}

	root.AddCommand(cmd)

	TrackPlugin(cmd, "1.2.3")

	assert.True(t, IsPluginCommand(cmd))

	event, ok := EventFor(cmd, nil)
	assert.True(t, ok)
	assert.Equal(t, "dr assist", event.EventType)
	assert.Equal(t, "1.2.3", event.EventProperties["plugin_version"])
}

func TestEventFor_UntrackedCommandReturnsFalse(t *testing.T) {
	cmd := &cobra.Command{Use: "untracked"}

	event, ok := EventFor(cmd, nil)
	assert.False(t, ok)
	assert.Empty(t, event.EventType)
	assert.False(t, IsPluginCommand(cmd))
}

func TestEventFor_NilCommand(t *testing.T) {
	event, ok := EventFor(nil, nil)
	assert.False(t, ok)
	assert.Empty(t, event.EventType)
	assert.False(t, IsPluginCommand(nil))
}

func TestFirstArg(t *testing.T) {
	assert.Empty(t, FirstArg(nil))
	assert.Empty(t, FirstArg([]string{}))
	assert.Equal(t, "first", FirstArg([]string{"first"}))
	assert.Equal(t, "first", FirstArg([]string{"first", "second"}))
}
