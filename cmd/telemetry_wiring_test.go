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
	"strings"
	"testing"

	"github.com/datarobot/cli/cmd/enclave"
	"github.com/datarobot/cli/cmd/workload"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedTrackedCommands enumerates every core command path that must be
// wired to fire a telemetry event. Plugin commands are wired at runtime by
// cmd/plugin/discovery.go and are intentionally omitted from this list.
//
// If you add or remove a tracked command, update this list AND the
// corresponding telemetry.Track* call at the command-construction site.
var expectedTrackedCommands = []string{
	"dr start",
	"dr dependency check",
	"dr dependency install",
	"dr run",
	"dr task",
	"dr auth set-url",
	"dr dotenv setup",
	"dr dotenv update",
	"dr dotenv validate",
	"dr component add",
	"dr component update",
	"dr template setup",
	"dr plugin install",
	"dr plugin uninstall",
	"dr plugin update",

	// Pipelines, workloads and artifacts are GA, so no feature gate hides them
	// from the live RootCmd and they need no separate standalone list. The two
	// workload leaves still behind a gate are checked further down.
	"dr pipeline create",
	"dr pipeline get",
	"dr pipeline clone",
	"dr pipeline list",
	"dr pipeline update",
	"dr pipeline delete",
	"dr pipeline lock",
	"dr pipeline version get",
	"dr pipeline version list",
	"dr pipeline graph",
	"dr pipeline source",
	"dr pipeline task get",
	"dr pipeline input create",
	"dr pipeline input get",
	"dr pipeline input list",
	"dr pipeline input update",
	"dr pipeline input delete",
	"dr pipeline run create",
	"dr pipeline run get",
	"dr pipeline run list",
	"dr pipeline run status",
	"dr pipeline run cancel",
	"dr pipeline run task get",
	"dr pipeline run task list",
	"dr pipeline run task logs",
	"dr pipeline run task result",
	"dr pipeline schedule create",
	"dr pipeline schedule get",
	"dr pipeline schedule list",
	"dr pipeline schedule update",
	"dr pipeline schedule delete",
	"dr pipeline image create",
	"dr pipeline image get",
	"dr pipeline image list",
	"dr pipeline image update",
	"dr pipeline image delete",
	"dr pipeline image version delete",
	"dr pipeline image version logs",

	"dr workload create",
	"dr workload get",
	"dr workload list",
	"dr workload delete",
	"dr workload start",
	"dr workload stop",
	"dr workload status",
	"dr workload endpoint",
	"dr workload logs",

	"dr artifact create",
	"dr artifact get",
	"dr artifact list",
	"dr artifact delete",
	"dr artifact lock",
	"dr artifact build create",
	"dr artifact build get",
	"dr artifact build list",
	"dr artifact build logs",
	"dr artifact code init",
	"dr artifact code sync",
	"dr artifact code versions",
	"dr artifact code checkout",
}

// trackedSubtrees are the command groups every leaf of which must fire a
// telemetry event.
//
// expectedTrackedCommands can only say that the commands somebody listed are
// tracked; it cannot say which ones nobody listed, and a leaf missing from a
// hand-kept list is invisible to a loop over that list. That is how
// `dr artifact code checkout` reached general availability untracked. These
// groups are walked instead, so the next leaf added under one of them fails
// here until it is wired.
//
// Only API groups are walked. Elsewhere in the tree an untracked leaf is
// routine (`dr auth check`, `dr self version`), so a walk would have to carry
// a list of exemptions, which is the same hand-kept list with the burden of
// proof reversed.
var trackedSubtrees = []string{
	"dr workload",
	"dr artifact",
	"dr pipeline",
}

// TestTelemetryWiring_AllCoreCommandsTracked walks the static command tree
// rooted at RootCmd and asserts each entry in expectedTrackedCommands has
// the "telemetry" annotation set by telemetry.Track / TrackWith.
func TestTelemetryWiring_AllCoreCommandsTracked(t *testing.T) {
	for _, path := range expectedTrackedCommands {
		t.Run(path, func(t *testing.T) {
			cmd := findCommandByPath(RootCmd.Command, path)
			require.NotNilf(t, cmd, "command %q not found in static command tree", path)

			assert.Containsf(t, cmd.Annotations, "telemetry",
				"command %q must be wired to telemetry via telemetry.Track / TrackWith", path)
		})
	}
}

// TestTelemetryWiring_EveryLeafOfTrackedSubtreesTracked walks each group in
// trackedSubtrees and asserts every leaf under it carries the telemetry
// annotation, whether or not anybody remembered to list it above.
//
// Leaves rather than runnable commands: a group command is not expected to
// fire an event, and the two are told apart by having children rather than by
// what they do when run, which is how `dr artifact build` (no children of its
// own to speak for it) would otherwise be read as a missing wiring.
func TestTelemetryWiring_EveryLeafOfTrackedSubtreesTracked(t *testing.T) {
	for _, path := range trackedSubtrees {
		root := findCommandByPath(RootCmd.Command, path)
		require.NotNilf(t, root, "command %q not found in static command tree", path)

		for _, leaf := range leafCommands(root) {
			t.Run(leaf.CommandPath(), func(t *testing.T) {
				assert.Containsf(t, leaf.Annotations, "telemetry",
					"command %q must be wired to telemetry via telemetry.Track / TrackWith, "+
						"and listed in expectedTrackedCommands", leaf.CommandPath())
			})
		}
	}
}

// leafCommands returns every descendant of root that has no subcommands of
// its own, root included when it has none.
func leafCommands(root *cobra.Command) []*cobra.Command {
	children := root.Commands()
	if len(children) == 0 {
		return []*cobra.Command{root}
	}

	var leaves []*cobra.Command

	for _, child := range children {
		leaves = append(leaves, leafCommands(child)...)
	}

	return leaves
}

// expectedGatedWorkloadTrackedCommands enumerates the `dr workload` leaves
// still behind DATAROBOT_CLI_FEATURE_WORKLOAD. cli.CommandAdder leaves them
// out of the tree while the variable is unset (the default in CI), so the
// test below sets it and walks a freshly-built subtree from workload.Cmd()
// rather than the global RootCmd.
//
// Paths are relative to workload.Cmd() (no "dr" prefix) because
// findCommandByPath matches against the root's Name(), which is "workload"
// for the standalone subtree.
var expectedGatedWorkloadTrackedCommands = []string{
	"workload config",
	"workload up",
}

// TestTelemetryWiring_GatedWorkloadCommandsTracked enables the workload gate,
// builds the subtree and asserts each gated leaf has the "telemetry"
// annotation set by telemetry.Track / TrackWith.
func TestTelemetryWiring_GatedWorkloadCommandsTracked(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_FEATURE_WORKLOAD", "true")

	workloadRoot := workload.Cmd()

	for _, path := range expectedGatedWorkloadTrackedCommands {
		t.Run("dr "+path, func(t *testing.T) {
			cmd := findCommandByPath(workloadRoot, path)
			require.NotNilf(t, cmd, "command %q not found in workload subtree", path)

			assert.Containsf(t, cmd.Annotations, "telemetry",
				"command %q must be wired to telemetry via telemetry.Track / TrackWith", path)
		})
	}
}

// expectedEnclaveTrackedCommands enumerates leaf commands under `dr enclave`
// that must be wired to fire a telemetry event. Like the workload subtree,
// `dr enclave` is hidden from the live RootCmd by cli.CommandAdder when
// DATAROBOT_CLI_FEATURE_ENCLAVE is unset (the default in CI), so this test
// walks a freshly-built subtree produced by enclave.Cmd().
var expectedEnclaveTrackedCommands = []string{
	"enclave register",
	"enclave get",
	"enclave list",
	"enclave deactivate",
	"enclave reactivate",
	"enclave delete",
	"enclave access grant",
	"enclave access revoke",
	"enclave access list",
	"enclave access show",
	"enclave permission grant",
	"enclave permission revoke",
	"enclave permission list",
	"enclave permission show",
}

// TestTelemetryWiring_AllEnclaveCommandsTracked walks the enclave subtree
// (built via enclave.Cmd() to bypass the feature-gate filter in
// cli.CommandAdder) and asserts each entry has the "telemetry" annotation set
// by telemetry.Track / TrackWith.
func TestTelemetryWiring_AllEnclaveCommandsTracked(t *testing.T) {
	enclaveRoot := enclave.Cmd()

	for _, path := range expectedEnclaveTrackedCommands {
		t.Run("dr "+path, func(t *testing.T) {
			cmd := findCommandByPath(enclaveRoot, path)
			require.NotNilf(t, cmd, "command %q not found in enclave subtree", path)

			assert.Containsf(t, cmd.Annotations, "telemetry",
				"command %q must be wired to telemetry via telemetry.Track / TrackWith", path)
		})
	}
}

// findCommandByPath locates a descendant command by its full CommandPath
// (e.g., "dr dotenv setup"). It returns nil if no such command exists.
func findCommandByPath(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Split(path, " ")
	if len(parts) == 0 || parts[0] != root.Name() {
		return nil
	}

	current := root

	for _, name := range parts[1:] {
		next := childByName(current, name)
		if next == nil {
			return nil
		}

		current = next
	}

	return current
}

// childByName returns the immediate child command whose Name() (or any
// alias) matches name, or nil if none.
func childByName(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}

		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}

	return nil
}
