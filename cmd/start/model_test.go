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

package start

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckSelfVersion_EmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dr-test-*")
	assert.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	originalDir, err := os.Getwd()
	assert.NoError(t, err)

	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	msg := checkSelfVersion(nil)

	completeMsg, ok := msg.(stepCompleteMsg)
	assert.True(t, ok)
	assert.False(t, completeMsg.selfUpdate)
}

func TestCheckSelfVersion_WithVersionRequirement(t *testing.T) {
	// Note: This test verifies the logic when a version requirement exists,
	// but during development (version.Version == "dev"), SufficientSelfVersion
	// always returns true, so we expect no update prompt.
	// This test ensures the code path works correctly in dev mode.

	tmpDir, err := os.MkdirTemp("", "dr-test-*")
	assert.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	// Create .datarobot/cli for versions.yaml
	drDir := filepath.Join(tmpDir, ".datarobot", "cli")
	err = os.MkdirAll(drDir, 0755)
	assert.NoError(t, err)

	// Create .datarobot/answers so FindRepoRoot recognizes it as a repo
	answersDir := filepath.Join(tmpDir, ".datarobot", "answers")
	err = os.MkdirAll(answersDir, 0755)
	assert.NoError(t, err)

	versionsYaml := `dr:
  name: DataRobot CLI
  minimum-version: "999.999.999"
`
	err = os.WriteFile(filepath.Join(drDir, "versions.yaml"), []byte(versionsYaml), 0644)
	assert.NoError(t, err)

	originalDir, err := os.Getwd()
	assert.NoError(t, err)

	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	msg := checkSelfVersion(nil)

	completeMsg, ok := msg.(stepCompleteMsg)
	assert.True(t, ok)

	// In dev mode, version check always passes, so no update prompt
	assert.False(t, completeMsg.selfUpdate, "In dev mode, should not prompt for update")
	assert.False(t, completeMsg.waiting)
}
