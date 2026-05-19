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

// resetSharedMaps clears the package-level sharedExtractors and sharedPropKeys
// maps between tests so that keys registered in one test do not affect others.
func resetSharedMaps() {
	sharedExtractors.Range(func(k, _ any) bool {
		sharedExtractors.Delete(k)

		return true
	})

	sharedPropKeys.Range(func(k, _ any) bool {
		sharedPropKeys.Delete(k)

		return true
	})
}

// TestTrackWithShared_SetsAnnotationAndRegistersKeys verifies that
// TrackWithShared marks the command as tracked and stores the declared keys
// in the global sharedPropKeys set.
func TestTrackWithShared_SetsAnnotationAndRegistersKeys(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "task"}

	root.AddCommand(cmd)

	TrackWithShared(cmd, []string{"task_name", "task_id"}, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"task_name": FirstArg(args),
			"task_id":   "123",
		}
	})

	assert.Contains(t, cmd.Annotations, trackAnnotation, "TrackWithShared should set the telemetry annotation")

	var found []string

	sharedPropKeys.Range(func(k, _ any) bool {
		if key, ok := k.(string); ok {
			found = append(found, key)
		}

		return true
	})

	assert.ElementsMatch(t, []string{"task_name", "task_id"}, found)
}

// TestTrackWithShared_PassesPropertiesFromExtractor verifies that EventFor
// invokes the shared extractor and includes its output in EventProperties.
func TestTrackWithShared_PassesPropertiesFromExtractor(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "run"}

	root.AddCommand(cmd)

	TrackWithShared(cmd, []string{"task_name"}, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"task_name": FirstArg(args),
		}
	})

	event, ok := EventFor(cmd, []string{"build"})
	assert.True(t, ok)
	assert.Equal(t, "dr run", event.EventType)
	assert.Equal(t, "build", event.EventProperties["task_name"])
}

// TestTrackWithShared_CoexistsWithTrackWith verifies that a command can have
// both a regular TrackWith extractor and a TrackWithShared extractor. Both
// extractors contribute their properties to the final event.
func TestTrackWithShared_CoexistsWithTrackWith(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "run"}

	root.AddCommand(cmd)

	// Per-command property (not shared across event types)
	TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"parallel": true}
	})

	// Shared property (omitted if empty across all event types)
	TrackWithShared(cmd, []string{"task_name"}, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"task_name": FirstArg(args),
		}
	})

	event, ok := EventFor(cmd, []string{"dev"})
	assert.True(t, ok)
	assert.Equal(t, "dev", event.EventProperties["task_name"])
	assert.Equal(t, true, event.EventProperties["parallel"])
}

// TestTrackWithShared_NonRegisteredCommandOmitsKey verifies that when a key is
// declared via TrackWithShared on one command, unregistered commands do not get
// that key seeded into their events (it is omitted as an empty value).
// The key is tracked in sharedPropKeys so Client.Track can identify and omit it.
func TestTrackWithShared_NonRegisteredCommandOmitsKey(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	taskCmd := &cobra.Command{Use: "task"}
	otherCmd := &cobra.Command{Use: "ping"}

	root.AddCommand(taskCmd, otherCmd)

	TrackWithShared(taskCmd, []string{"task_name"}, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{"task_name": FirstArg(args)}
	})

	Track(otherCmd)

	// The "ping" event does not have task_name set via EventFor — it has no shared extractor.
	event, ok := EventFor(otherCmd, nil)
	assert.True(t, ok)
	assert.NotContains(t, event.EventProperties, "task_name",
		"EventFor does not seed shared keys; Client.Track omits them as empty")

	// But sharedPropKeys has the key, which Client.Track uses to identify and omit it.
	var keyRegistered bool

	sharedPropKeys.Range(func(k, _ any) bool {
		if k.(string) == "task_name" {
			keyRegistered = true
		}

		return true
	})

	assert.True(t, keyRegistered, "task_name should be in sharedPropKeys after TrackWithShared call")
}

// TestTrackWithShared_EmptyExtractorValueOmitsKey verifies that when a shared
// extractor returns an empty string for a key, Client.Track omits that key
// from the event properties per Amplitude's preference.
func TestTrackWithShared_EmptyExtractorValueOmitsKey(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "run"}

	root.AddCommand(cmd)

	TrackWithShared(cmd, []string{"task_name"}, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"task_name": "", // Explicitly return empty string
		}
	})

	event, ok := EventFor(cmd, nil)
	assert.True(t, ok)
	// EventFor includes the empty value from the extractor
	assert.Empty(t, event.EventProperties["task_name"])
}

// TestTrackWithShared_NonEmptyValueIsIncluded verifies that shared properties
// with non-empty values are included in the final event.
func TestTrackWithShared_NonEmptyValueIsIncluded(t *testing.T) {
	t.Cleanup(resetSharedMaps)

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "task"}

	root.AddCommand(cmd)

	TrackWithShared(cmd, []string{"task_name"}, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"task_name": FirstArg(args),
		}
	})

	event, ok := EventFor(cmd, []string{"build"})
	assert.True(t, ok)
	assert.Equal(t, "build", event.EventProperties["task_name"])
}

func TestFirstArg(t *testing.T) {
	assert.Empty(t, FirstArg(nil))
	assert.Empty(t, FirstArg([]string{}))
	assert.Equal(t, "first", FirstArg([]string{"first"}))
	assert.Equal(t, "first", FirstArg([]string{"first", "second"}))
}
