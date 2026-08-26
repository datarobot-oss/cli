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

package cmd

import (
	"sync"
	"testing"

	"github.com/amplitude/analytics-go/amplitude"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingClient is a test double for amplitude.Client that records Track
// calls; every other method is a no-op.
type countingClient struct {
	mu     sync.Mutex
	events []amplitude.Event
}

func (c *countingClient) Track(event amplitude.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, event)
}

func (c *countingClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.events)
}

func (c *countingClient) Identify(amplitude.Identify, amplitude.EventOptions)                      {}
func (c *countingClient) GroupIdentify(string, string, amplitude.Identify, amplitude.EventOptions) {}
func (c *countingClient) SetGroup(string, []string, amplitude.EventOptions)                        {}
func (c *countingClient) Revenue(amplitude.Revenue, amplitude.EventOptions)                        {}
func (c *countingClient) Flush()                                                                   {}
func (c *countingClient) Shutdown()                                                                {}
func (c *countingClient) Add(amplitude.Plugin)                                                     {}
func (c *countingClient) Remove(string)                                                            {}

func (c *countingClient) Config() amplitude.Config { return amplitude.Config{} }

// executeStubOnce builds a fully-isolated tree whose telemetry client is
// backed by client, attaches a tracked stub command, and executes it once.
func executeStubOnce(t *testing.T, client *countingClient) {
	t.Helper()

	root := NewIsolatedRootFactory(
		WithTelemetryClient(func(_ *telemetry.CommonProperties) *telemetry.Client {
			return telemetry.NewTestClient(client, nil)
		}),
	).Build()

	stub := &cobra.Command{
		Use:  "stub",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}

	telemetry.Track(stub)
	root.AddCommand(stub)

	root.SetArgs([]string{"stub"})
	require.NoError(t, root.Execute())
}

// TestStaleFinalizersDoNotRetrack verifies that executing a second
// factory-built tree does not cause the first tree's finalizer to re-track
// its stale event. cobra.OnFinalize appends to a process-global list that is
// replayed in full on every Execute; the factory's generation guard
// suppresses those stale replays so each execute tracks exactly one event.
func TestStaleFinalizersDoNotRetrack(t *testing.T) {
	clientA := &countingClient{}
	executeStubOnce(t, clientA)
	assert.Equal(t, 1, clientA.count(), "first execute should track exactly one event")

	clientB := &countingClient{}
	executeStubOnce(t, clientB)

	assert.Equal(t, 1, clientB.count(), "second execute should track exactly one event")
	assert.Equal(t, 1, clientA.count(), "stale finalizer from first execute must not re-track")
}
