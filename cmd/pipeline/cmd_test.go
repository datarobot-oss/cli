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

package pipeline

import (
	"testing"

	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCmd_BasicMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "pipeline", cmd.Use)
	assert.Equal(t, "core", cmd.GroupID)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

func TestCmd_HasAlias(t *testing.T) {
	cmd := Cmd()

	assert.Contains(t, cmd.Aliases, "pipelines")
}

func TestCmd_FeatureGate(t *testing.T) {
	cmd := Cmd()

	gate, ok := cmd.Annotations[features.AnnotationKey]
	assert.True(t, ok, "expected feature-gate annotation to be set")
	assert.Equal(t, "pipeline", gate)
}

func TestCmd_IsGroupOnly(t *testing.T) {
	cmd := Cmd()

	assert.Nil(t, cmd.RunE, "pipeline is a group command and should not have a RunE")
}

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	cmd := Cmd()

	want := map[string]bool{
		"create":  false,
		"get":     false,
		"list":    false,
		"update":  false,
		"delete":  false,
		"lock":    false,
		"version": false,
		"graph":   false,
		"run":     false,
	}

	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}

	for name, found := range want {
		assert.True(t, found, "expected subcommand %q to be registered", name)
	}
}

func TestCmd_VersionHasSubcommands(t *testing.T) {
	cmd := Cmd()

	var versionCmd *cobra.Command

	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			versionCmd = sub

			break
		}
	}

	assert.NotNil(t, versionCmd, "version subcommand must be registered")

	want := map[string]bool{
		"get":  false,
		"list": false,
	}

	for _, sub := range versionCmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}

	for name, found := range want {
		assert.True(t, found, "expected version subcommand %q to be registered", name)
	}
}
