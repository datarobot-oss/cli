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
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/amplitude/analytics-go/amplitude"
	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAmplitude is a minimal amplitude.Client that records Track calls.
// Embedding the interface leaves every other method nil: the code under test
// only ever calls Track.
type recordingAmplitude struct {
	amplitude.Client

	events []types.Event
}

func (r *recordingAmplitude) Track(event types.Event) {
	r.events = append(r.events, event)
}

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

func TestTrackCommandFromContext_EnqueuesEventForTrackedCommand(t *testing.T) {
	amp := &recordingAmplitude{}
	ctx := context.WithValue(context.Background(), ClientContextKey{}, NewTestClient(amp, nil))

	root := &cobra.Command{Use: "dr"}
	cmd := &cobra.Command{Use: "assist"}

	root.AddCommand(cmd)

	TrackPlugin(cmd, "1.2.3")

	TrackCommandFromContext(ctx, cmd, nil)

	require.Len(t, amp.events, 1, "a tracked command should enqueue exactly one event")
	assert.Equal(t, "dr assist", amp.events[0].EventType)
	assert.Equal(t, "1.2.3", amp.events[0].EventProperties["plugin_version"])
}

func TestTrackCommandFromContext_SkipsUntrackedCommand(t *testing.T) {
	amp := &recordingAmplitude{}
	ctx := context.WithValue(context.Background(), ClientContextKey{}, NewTestClient(amp, nil))

	TrackCommandFromContext(ctx, &cobra.Command{Use: "untracked"}, nil)

	assert.Empty(t, amp.events, "a command without a telemetry annotation must not enqueue an event")
}

func TestTrackCommandFromContext_NoClientInContextIsNoOp(t *testing.T) {
	cmd := &cobra.Command{Use: "assist"}

	TrackPlugin(cmd, "1.2.3")

	assert.NotPanics(t, func() {
		TrackCommandFromContext(context.Background(), cmd, nil)
	})
}

// printingAmplitude reports Track calls on stdout so a parent test can observe
// events queued in a child process that then calls os.Exit. Shutdown is a
// no-op because Client.Flush calls it.
type printingAmplitude struct {
	amplitude.Client
}

func (printingAmplitude) Track(event types.Event) {
	fmt.Printf("tracked:%s plugin_version:%v\n", event.EventType, event.EventProperties["plugin_version"])
}

func (printingAmplitude) Shutdown() {}

const exitCommandChildEnv = "TELEMETRY_TEST_EXIT_COMMAND_CHILD"

// ExitCommand ends in os.Exit, so the test re-runs this test binary and
// inspects its exit code and output.
func TestExitCommand_QueuesEventThenExitsWithCode(t *testing.T) {
	if os.Getenv(exitCommandChildEnv) == "1" {
		root := &cobra.Command{Use: "dr"}
		cmd := &cobra.Command{Use: "assist"}

		root.AddCommand(cmd)
		TrackPlugin(cmd, "1.2.3")

		cmd.SetContext(context.WithValue(context.Background(), ClientContextKey{}, NewTestClient(printingAmplitude{}, nil)))

		ExitCommand(cmd, nil, 7)

		return
	}

	child := exec.Command(os.Args[0], "-test.run=^TestExitCommand_QueuesEventThenExitsWithCode$") //nolint:gosec // re-runs this test binary

	child.Env = append(os.Environ(), exitCommandChildEnv+"=1")

	out, err := child.Output()

	var exitErr *exec.ExitError

	require.ErrorAs(t, err, &exitErr, "child should exit non-zero")
	assert.Equal(t, 7, exitErr.ExitCode())
	assert.Equal(t, "tracked:dr assist plugin_version:1.2.3\n", string(out),
		"the event must be queued before the process exits")
}

func TestFirstArg(t *testing.T) {
	assert.Empty(t, FirstArg(nil))
	assert.Empty(t, FirstArg([]string{}))
	assert.Equal(t, "first", FirstArg([]string{"first"}))
	assert.Equal(t, "first", FirstArg([]string{"first", "second"}))
}
