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

package workload

import (
	"testing"

	"github.com/datarobot/cli/internal/features"
	"github.com/stretchr/testify/assert"
)

func TestCmd_HasAlias(t *testing.T) {
	cmd := Cmd()

	assert.Contains(t, cmd.Aliases, "wl")
}

func TestCmd_NotFeatureGated(t *testing.T) {
	cmd := Cmd()

	// The workload root is generally available: it must carry no feature-gate
	// annotation, or cli.CommandAdder drops it from the root command tree
	// unless DATAROBOT_CLI_FEATURE_WORKLOAD is set. The gate now lives on two
	// of its subcommands instead; see TestCmd_GatesUpAndConfig below and
	// TestWorkloadCommandPresentByDefault in cmd/root_test.go.
	assert.NotContains(t, cmd.Annotations, features.AnnotationKey,
		"workload is GA and must not carry a %q annotation", features.AnnotationKey)
}

// TestCmd_GatesUpAndConfig pins which subcommands the workload gate still
// covers: `up` and `config` are registered only when
// DATAROBOT_CLI_FEATURE_WORKLOAD is set, every other verb always. The gate is
// applied at registration inside Cmd(), so building the subtree is enough.
func TestCmd_GatesUpAndConfig(t *testing.T) {
	always := []string{"create", "delete", "endpoint", "get", "list", "logs", "start", "status", "stop"}
	gated := []string{"config", "up"}

	tests := []struct {
		name      string
		gate      string
		wantGated bool
	}{
		{name: "gate unset leaves up and config out", gate: "", wantGated: false},
		{name: "gate set registers up and config", gate: "true", wantGated: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATAROBOT_CLI_FEATURE_WORKLOAD", tt.gate)

			registered := map[string]bool{}

			for _, sub := range Cmd().Commands() {
				registered[sub.Name()] = true
			}

			for _, name := range always {
				assert.True(t, registered[name], "dr workload %s must always be registered", name)
			}

			for _, name := range gated {
				assert.Equal(t, tt.wantGated, registered[name],
					"dr workload %s registered with DATAROBOT_CLI_FEATURE_WORKLOAD=%q", name, tt.gate)
			}
		})
	}
}
