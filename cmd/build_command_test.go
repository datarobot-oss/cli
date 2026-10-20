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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildCommandGatedByDefault verifies that `dr build` stays out of the
// command tree until the build-studio feature gate is enabled. The env var is
// neutralized so an ambient value (e.g. from the repo's .env) cannot leak in.
func TestBuildCommandGatedByDefault(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_FEATURE_BUILD_STUDIO", "")

	root := newIsolatedRootCmd()

	assert.Nil(t, findCommandByPath(root.Command, "dr build"),
		"build command should be hidden while the build-studio gate is off")
}

// TestBuildStudioPresentWhenGateEnabled verifies that enabling the
// build-studio gate registers `dr build` with its `studio` subcommand.
func TestBuildStudioPresentWhenGateEnabled(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_FEATURE_BUILD_STUDIO", "true")

	root := newIsolatedRootCmd()

	buildCmd := findCommandByPath(root.Command, "dr build")

	require.NotNil(t, buildCmd, "build command should be present with the build-studio gate on")
	assert.Equal(t, "core", buildCmd.GroupID)

	studioCmd := findCommandByPath(root.Command, "dr build studio")

	require.NotNil(t, studioCmd, "studio subcommand should be registered under build")
	assert.NotNil(t, studioCmd.Flags().Lookup("yes"), "studio should expose the --yes flag")
}
