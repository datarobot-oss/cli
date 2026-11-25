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

// TestBuildStudioRegistered verifies that `dr build-studio` is registered as a
// top-level core command, and that the earlier `dr build studio` spelling is
// not registered.
func TestBuildStudioRegistered(t *testing.T) {
	root := newIsolatedRootCmd()

	studioCmd := findCommandByPath(root.Command, "dr build-studio")

	require.NotNil(t, studioCmd, "build-studio command should be registered")
	assert.Equal(t, "core", studioCmd.GroupID)
	assert.NotNil(t, studioCmd.Flags().Lookup("yes"), "build-studio should expose the --yes flag")

	assert.Nil(t, findCommandByPath(root.Command, "dr build"),
		"there is no `dr build` parent; Build Studio is a single top-level command")
}
