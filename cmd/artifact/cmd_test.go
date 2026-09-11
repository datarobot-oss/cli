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

package artifact

import (
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findChild returns the named direct subcommand, or nil.
func findChild(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}

	return nil
}

// TestCmd_WorkloadGateHidesDoctor pins the gate-off behavior: without
// DATAROBOT_CLI_FEATURE_WORKLOAD the whole artifact tree (doctor included) is
// filtered out at registration time; with it, doctor is discoverable under
// `artifact code`.
func TestCmd_WorkloadGateHidesDoctor(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_FEATURE_WORKLOAD", "")

	gatedRoot := &cli.CommandAdder{Command: &cobra.Command{Use: "root"}}
	gatedRoot.AddCommand(Cmd())

	assert.Nil(t, findChild(gatedRoot.Command, "artifact"),
		"gate off: the artifact tree (and doctor with it) is not registered")

	t.Setenv("DATAROBOT_CLI_FEATURE_WORKLOAD", "true")

	openRoot := &cli.CommandAdder{Command: &cobra.Command{Use: "root"}}
	openRoot.AddCommand(Cmd())

	artifactCmd := findChild(openRoot.Command, "artifact")

	require.NotNil(t, artifactCmd, "gate on: artifact registered")

	codeCmd := findChild(artifactCmd, "code")

	require.NotNil(t, codeCmd, "gate on: artifact code registered")

	doctorCmd := findChild(codeCmd, "doctor")

	require.NotNil(t, doctorCmd, "gate on: doctor inherits the artifact tree's gate")

	assert.Empty(t, doctorCmd.Annotations["feature-gate"],
		"doctor needs no gate of its own; the parent carries it")
}
