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
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/amplitude/analytics-go/amplitude"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/testutil"
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

// TestPersistentPreRunStampsInteractionMode verifies that persistentPreRun
// stamps the merged non-interactive signal onto the collected telemetry
// props: a --yes invocation must report NonInteractive even though the
// collector only seeds the env-var signal. Without the StampInteractionMode
// call in persistentPreRun, --yes-only invocations would report
// non_interactive=false to analytics.
func TestPersistentPreRunStampsInteractionMode(t *testing.T) {
	// Seed props the way the production collector would for an interactive
	// run: NonInteractive unset. persistentPreRun must overwrite it with the
	// merged signal once flags are parsed.
	props := &telemetry.CommonProperties{}

	root := NewIsolatedRootFactory(
		WithTelemetryProps(func() *telemetry.CommonProperties { return props }),
	).Build()

	stub := &cobra.Command{
		Use:  "stub",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}

	stub.Flags().Bool(cli.YesFlagName, false, "")
	root.AddCommand(stub)

	root.SetArgs([]string{"stub", "--yes"})
	require.NoError(t, root.Execute())

	assert.True(t, props.NonInteractive, "persistentPreRun must stamp non_interactive for --yes invocations")
}

// buildProfileAwareTree builds a tree with the real ConfigInitializer and
// ViperBinder (so --profile/--config are actually bound to viper and
// drconfig.yaml is actually read), while keeping every other dependency
// isolated the way NewIsolatedRootFactory does.
func buildProfileAwareTree() *cli.CommandAdder {
	return NewRootFactory(
		WithConfigInitializer(defaultConfigInitializer),
		WithViperBinder(bindViperFlags),
		WithTLSSetup(func(_ *cobra.Command) error { return nil }),
		WithTelemetryProps(func() *telemetry.CommonProperties { return nil }),
		WithTelemetryClient(func(_ *telemetry.CommonProperties) *telemetry.Client {
			return telemetry.NewTestClient(nil, nil)
		}),
		WithAnimation(func() {}),
		WithPluginRegistrar(func(_ *cobra.Command) {}),
	).Build()
}

// writeProfileConfig writes a drconfig.yaml under a fresh temp home dir with
// a default profile plus one named "eu-mtsaas", and points HOME/XDG at it.
func writeProfileConfig(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	raw := `endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drconfig.yaml"), []byte(raw), 0o600))
}

func addNoopStub(root *cli.CommandAdder) {
	stub := &cobra.Command{
		Use:  "stub",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}

	root.AddCommand(stub)
}

func TestProfileFlag_RegisteredAndUniversal(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	root := buildProfileAwareTree()

	flag := root.PersistentFlags().Lookup(config.ProfileKey)
	require.NotNil(t, flag, "--profile must be registered as a persistent flag")

	suffixes, ok := flag.Annotations[config.UniversalAnnotationKey]
	require.True(t, ok, "--profile must carry the universal annotation so it forwards to plugins")
	assert.Equal(t, []string{"PROFILE"}, suffixes)
}

func TestProfileFlag_FlagBeatsEnv(t *testing.T) {
	writeProfileConfig(t)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	t.Setenv("DATAROBOT_CLI_PROFILE", "does-not-exist")

	root := buildProfileAwareTree()
	addNoopStub(root)

	root.SetArgs([]string{"--profile", "eu-mtsaas", "stub"})
	require.NoError(t, root.Execute(), "the explicit --profile flag must win over DATAROBOT_CLI_PROFILE")

	assert.Equal(t, "https://app.eu.datarobot.com/api/v2", viperx.GetString(config.DataRobotURL))
}

func TestProfileFlag_UnknownProfileFailsWithCandidateList(t *testing.T) {
	writeProfileConfig(t)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	root := buildProfileAwareTree()
	addNoopStub(root)

	root.SetArgs([]string{"--profile", "nope", "stub"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nope"`)
	assert.Contains(t, err.Error(), "eu-mtsaas")
}

func TestProfileFlag_CreateAnnotationSurvivesUnknownProfile(t *testing.T) {
	writeProfileConfig(t)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	root := buildProfileAwareTree()

	var sawEndpoint, sawToken string

	creator := &cobra.Command{
		Use: "creator",
		Annotations: map[string]string{
			config.ProfileCreateAnnotationKey: "true",
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			sawEndpoint = viperx.GetString(config.DataRobotURL)
			sawToken = viperx.GetString(config.DataRobotAPIKey)

			return nil
		},
	}
	root.AddCommand(creator)

	root.SetArgs([]string{"--profile", "brand-new", "creator"})
	require.NoError(t, root.Execute(), "a profile-creating command must survive an unknown --profile")

	assert.Empty(t, sawEndpoint, "credentials must be cleared so a new profile doesn't inherit the default's endpoint")
	assert.Empty(t, sawToken, "credentials must be cleared so a new profile doesn't inherit the default's token")
}
