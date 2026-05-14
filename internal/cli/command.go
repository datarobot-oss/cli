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

package cli

import (
	"errors"

	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

// ErrSilent is returned by RunE implementations that have already printed their
// own user-facing error message. Pair it with SilenceErrors: true on the
// cobra.Command (or set cmd.SilenceErrors = true before returning) so that
// cobra does not echo an additional "Error: ..." line to stderr.
// main.go will still call telemetry.Exit(1) for any non-nil error as normal.
var ErrSilent = errors.New("silent error")

// CommandAdder wraps cobra.Command and overrides AddCommand to filter gated commands.
// It allows commands with disabled feature gates to never be added to the command tree.
// This wrapper clarifies that the root command itself is not gated—it just intelligently
// adds its children, filtering out those with disabled feature gates.
type CommandAdder struct {
	*cobra.Command
}

// AddCommand adds commands to this command, skipping any that have a disabled feature gate.
// Gated commands (those with a feature-gate annotation) are filtered at registration time,
// never making it into the command tree if their feature is not enabled.
func (gc *CommandAdder) AddCommand(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		if gate, ok := cmd.Annotations[features.AnnotationKey]; ok && !features.Enabled(gate) {
			continue
		}

		gc.Command.AddCommand(cmd)
	}
}
