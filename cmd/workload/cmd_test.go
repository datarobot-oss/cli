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
	// unless DATAROBOT_CLI_FEATURE_WORKLOAD_ALPHA is set.
	assert.NotContains(t, cmd.Annotations, features.AnnotationKey,
		"workload is GA and must not carry a %q annotation", features.AnnotationKey)
}

// Every verb shipped so far is released: with the alpha gate unset, all of
// them are registered, config, up and promote included. Scripts that never
// set the gate, or set the name it had before, must keep finding them.
func TestCmd_RegistersEveryVerbWithoutTheGate(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_FEATURE_WORKLOAD_ALPHA", "")

	registered := map[string]bool{}

	for _, sub := range Cmd().Commands() {
		registered[sub.Name()] = true
	}

	for _, name := range []string{
		"config", "create", "delete", "diagnose", "endpoint", "events", "get", "list",
		"logs", "promote", "settings", "start", "status", "stop", "up",
	} {
		assert.True(t, registered[name], "dr workload %s must be registered without the gate", name)
	}
}
