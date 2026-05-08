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
