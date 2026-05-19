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
	"sync"

	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/spf13/cobra"
)

// trackAnnotation is the cobra annotation key set by Track / TrackWith / TrackPlugin
// to mark a command as one whose invocations should fire a telemetry event.
const trackAnnotation = "telemetry"

// pluginAnnotation is the cobra annotation key set by TrackPlugin to mark
// a command as a plugin command (used to populate the command_kind common property).
// For now this is a bool, because we may want to track when plugin commands are
// migrated to core commands.
const pluginAnnotation = "telemetry:plugin"

// annotationValue is the (otherwise unused) value stored under trackAnnotation
// and pluginAnnotation. Cobra annotations are map[string]string, but we only
// care whether the key is present.
const annotationValue = "true"

// PropExtractor returns a map of dynamic event-properties, basically a
// context, to be merged into the telemetry event fired for a command.
// It is invoked when we fire a telemetry event with the cobra command and
// the positional args passed to it.
// TODO I don't like that I have to use a map[string]any here instead of a
// struct or something more type safe. Refactor?
type PropExtractor func(cmd *cobra.Command, args []string) map[string]any

// commandProperties stores per-command PropExtractor closures registered via TrackWith /
// TrackPlugin. Keyed by *cobra.Command pointer.
var commandProperties sync.Map // map[*cobra.Command]PropExtractor

// sharedExtractors stores PropExtractor closures registered via TrackWithShared.
// Keyed by *cobra.Command pointer. Unlike commandProperties, the output of these
// extractors is also tracked by Client.Track to identify and omit empty-string values,
// ensuring consistency across all event types.
var sharedExtractors sync.Map // map[*cobra.Command]PropExtractor

// sharedPropKeys accumulates the set of all property keys declared by any
// TrackWithShared call. Client.Track uses this set to identify and omit empty-string
// values across all events, per Amplitude's preference to send only set properties.
var sharedPropKeys sync.Map // map[string]struct{}

// Track marks cmd as one whose invocation should fire a telemetry event.
// The event's EventType is derived from cmd.CommandPath() and no extra
// event properties are added. Common properties are merged at Track-time
// by Client.Track.
func Track(cmd *cobra.Command) {
	setAnnotation(cmd, trackAnnotation)
}

// TrackWith is Track plus a closure that contributes dynamic event properties.
// extract is invoked at event-firing time with the same cmd / args that
// Cobra passed to the command.
func TrackWith(cmd *cobra.Command, extract PropExtractor) {
	setAnnotation(cmd, trackAnnotation)

	if extract != nil {
		commandProperties.Store(cmd, extract)
	}
}

// TrackWithShared is like TrackWith but marks the extracted properties as
// "shared" — keys that should be consistently handled across telemetry events.
// The keys slice declares which map keys the extractor will produce; Client.Track
// uses this set to identify and omit empty-string values, per Amplitude's preference.
//
// Use TrackWithShared for properties that span a logical group of commands
// (e.g., task_name across dr task / dr run / dr task run). Use the regular
// TrackWith for command-specific properties that do not need to appear on
// unrelated events.
//
// Like TrackWith, the extractor is invoked at event-firing time (inside
// cobra.OnFinalize, after RunE completes), so closures over local variables
// updated during RunE work correctly.
func TrackWithShared(cmd *cobra.Command, keys []string, extract PropExtractor) {
	setAnnotation(cmd, trackAnnotation)

	if extract != nil {
		sharedExtractors.Store(cmd, extract)
	}

	// Register each declared key in the global shared-key set so Client.Track
	// can identify them and omit empty-string values across all events.
	for _, k := range keys {
		sharedPropKeys.Store(k, struct{}{})
	}
}

// TrackPlugin marks cmd as a plugin command and registers a closure that
// emits plugin_version as an event property. EventType remains
// cmd.CommandPath() (e.g., "dr assist"). The plugin annotation lets
// IsPluginCommand identify it for the command_kind common property.
func TrackPlugin(cmd *cobra.Command, version string) {
	setAnnotation(cmd, trackAnnotation)
	setAnnotation(cmd, pluginAnnotation)

	commandProperties.Store(cmd, PropExtractor(func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"plugin_version": version,
		}
	}))
}

// IsPluginCommand reports whether cmd was registered via TrackPlugin.
func IsPluginCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	_, ok := cmd.Annotations[pluginAnnotation]

	return ok
}

// mergeExtractorProps loads a PropExtractor from store keyed by cmd, invokes
// it, and merges the resulting properties into props. It is a no-op when no
// extractor is registered for cmd.
func mergeExtractorProps(store *sync.Map, cmd *cobra.Command, args []string, props map[string]any) {
	v, ok := store.Load(cmd)
	if !ok {
		return
	}

	extract, ok := v.(PropExtractor)
	if !ok || extract == nil {
		return
	}

	for k, val := range extract(cmd, args) {
		props[k] = val
	}
}

// EventFor returns the telemetry event to fire for cmd, if any. It is the
// single entry point used by the root command to translate a command
// invocation into an Amplitude event. Returns (_, false) when cmd has no
// telemetry annotation.
func EventFor(cmd *cobra.Command, args []string) (types.Event, bool) {
	if cmd == nil {
		return types.Event{}, false
	}

	if _, ok := cmd.Annotations[trackAnnotation]; !ok {
		return types.Event{}, false
	}

	event := types.Event{
		EventType:       cmd.CommandPath(),
		EventProperties: map[string]any{},
	}

	// Per-command properties (only appear on this event type).
	mergeExtractorProps(&commandProperties, cmd, args, event.EventProperties)

	// Shared properties. These keys are consistently handled across all event types
	// by Client.Track, which omits empty-string values per Amplitude's preference.
	mergeExtractorProps(&sharedExtractors, cmd, args, event.EventProperties)

	return event, true
}

// FirstArg returns the first element of args, or "" if args is empty.
// Convenience helper for PropExtractor closures.
func FirstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}

	return ""
}

// setAnnotation stores key=annotationValue on cmd, allocating the
// Annotations map if nil.
func setAnnotation(cmd *cobra.Command, key string) {
	if cmd == nil {
		return
	}

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	cmd.Annotations[key] = annotationValue
}
