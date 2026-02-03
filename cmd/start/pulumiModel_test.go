// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package start

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPulumiState_PulumiNotInstalled(t *testing.T) {
	// This test assumes pulumi is not in PATH with a name like "pulumi-nonexistent"
	// The checkPulumiState function should skip the check gracefully
	m := &Model{}
	msg := checkPulumiState(m)

	// When pulumi is not installed, should return stepCompleteMsg with defaults
	stepMsg, ok := msg.(stepCompleteMsg)
	assert.True(t, ok, "Expected stepCompleteMsg")
	assert.False(t, stepMsg.needPulumiLogin, "Should not need Pulumi login when not installed")
}

func TestPulumiLoginModel_InitialState(t *testing.T) {
	model := newPulumiLoginModel()

	assert.Equal(t, pulumiLoginScreenBackendSelection, model.currentScreen)
	assert.Equal(t, 0, model.selectedOption)
	assert.Len(t, model.options, 3)
	assert.Equal(t, "Login locally", model.options[0])
	assert.Equal(t, "Login to Pulumi Cloud", model.options[1])
	assert.Contains(t, model.options[2], "DIY")
}

func TestGenerateRandomPassphrase(t *testing.T) {
	passphrase, err := generateRandomPassphrase(32)
	require.NoError(t, err)
	assert.Len(t, passphrase, 32)

	// Generate another one to ensure they're different
	passphrase2, err := generateRandomPassphrase(32)
	require.NoError(t, err)
	assert.NotEqual(t, passphrase, passphrase2, "Generated passphrases should be unique")
}
