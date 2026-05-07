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

package dotenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary git repository with parakeet.yaml for testing
func setupTestRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir

	err := cmd.Run()
	require.NoError(t, err, "Failed to initialize git repository")

	// Create .datarobot directory
	datarobotDir := filepath.Join(repoDir, ".datarobot")

	err = os.MkdirAll(datarobotDir, 0o755)
	require.NoError(t, err, "Failed to create .datarobot directory")

	// Create parakeet.yaml with basic configuration
	parakeetYaml := `root:
  - env: TEST_VAR
    type: string
    default: "test_default"
    optional: true
    help: "A test variable"
`
	parakeetPath := filepath.Join(datarobotDir, "parakeet.yaml")

	err = os.WriteFile(parakeetPath, []byte(parakeetYaml), 0o600)
	require.NoError(t, err, "Failed to create parakeet.yaml")

	return repoDir
}

func TestSetupCmd_OutputFlag(t *testing.T) {
	// Reset viper to prevent leaking config
	viperx.Reset()

	repoDir := setupTestRepo(t)
	outputDir := t.TempDir()

	// Change to repository directory
	originalWd, _ := os.Getwd()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err := os.Chdir(repoDir)
	require.NoError(t, err)

	// Test the RunE function directly instead of executing the command
	// to avoid cobra command state issues in tests
	err = setupNonInteractive(repoDir, filepath.Join(outputDir, ".env"))
	require.NoError(t, err, "setupNonInteractive should succeed")

	// Verify .env file was created in the output directory
	dotenvPath := filepath.Join(outputDir, ".env")

	_, err = os.Stat(dotenvPath)
	require.NoError(t, err, "Expected .env file to exist in output directory")

	// Verify contents
	contents, err := os.ReadFile(dotenvPath)
	require.NoError(t, err)

	assert.Contains(t, string(contents), "TEST_VAR=\"test_default\"", "Expected .env to contain default value")
}

func TestSetupCmd_DefaultOutput(t *testing.T) {
	// Reset viper to prevent leaking config
	viperx.Reset()

	repoDir := setupTestRepo(t)

	// Change to repository directory
	originalWd, _ := os.Getwd()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err := os.Chdir(repoDir)
	require.NoError(t, err)

	// Test the RunE function directly - default output should use repo root
	err = setupNonInteractive(repoDir, filepath.Join(repoDir, ".env"))
	require.NoError(t, err, "setupNonInteractive should succeed")

	// Verify .env file was created in the repository root
	dotenvPath := filepath.Join(repoDir, ".env")

	_, err = os.Stat(dotenvPath)
	require.NoError(t, err, "Expected .env file to exist in repository root")

	// Verify contents
	contents, err := os.ReadFile(dotenvPath)
	require.NoError(t, err)

	assert.Contains(t, string(contents), "TEST_VAR=\"test_default\"", "Expected .env to contain default value")
}

func TestSetupCmd_OutputFlagCreatesDirectory(t *testing.T) {
	// Reset viper to prevent leaking config
	viperx.Reset()

	repoDir := setupTestRepo(t)
	outputDir := filepath.Join(t.TempDir(), "nested", "path", "that", "does", "not", "exist")

	// Change to repository directory
	originalWd, _ := os.Getwd()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err := os.Chdir(repoDir)
	require.NoError(t, err)

	// Test the RunE function directly with non-existent directory
	err = setupNonInteractive(repoDir, filepath.Join(outputDir, ".env"))
	require.NoError(t, err, "setupNonInteractive should succeed and create directory")

	// Verify .env file was created in the nested output directory
	dotenvPath := filepath.Join(outputDir, ".env")

	_, err = os.Stat(dotenvPath)
	require.NoError(t, err, "Expected .env file to exist in created output directory")

	// Verify the directory was created
	_, err = os.Stat(outputDir)
	require.NoError(t, err, "Expected output directory to be created")
}

func TestSetupCmd_OutputFlag_SkipsGitRepositorySearch(t *testing.T) {
	// Reset viper to prevent leaking config
	viperx.Reset()

	// Create a target DataRobot app directory with proper structure
	targetAppDir := setupTestRepo(t)

	// Create a completely separate directory that is NOT a DataRobot app
	nonAppDir := t.TempDir()
	// Ensure it doesn't have .datarobot structure
	_, err := os.Stat(filepath.Join(nonAppDir, ".datarobot"))
	require.Error(t, err, "Non-app directory should not have .datarobot folder")

	// Change to the NON-APP directory
	originalWd, _ := os.Getwd()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err = os.Chdir(nonAppDir)
	require.NoError(t, err)

	// Verify we are indeed in a non-app directory by checking ensureInRepo would fail
	_, err = ensureInRepo()
	require.Error(t, err, "ensureInRepo should fail when not in app directory")
	assert.Contains(t, err.Error(), "Not in git repository")

	// Now test that setupNonInteractive succeeds when using targetAppDir as the repo root
	// This proves we skipped the directory walk because we're not in an app directory
	dotenvPath := filepath.Join(targetAppDir, ".env")

	err = setupNonInteractive(targetAppDir, dotenvPath)
	require.NoError(t, err, "setupNonInteractive should succeed with explicit repo root even when not in app directory")

	// Verify .env file was created in the target directory
	_, err = os.Stat(dotenvPath)
	require.NoError(t, err, "Expected .env file to exist in target app directory")

	// Verify contents
	contents, err := os.ReadFile(dotenvPath)
	require.NoError(t, err)

	assert.Contains(t, string(contents), "TEST_VAR=\"test_default\"", "Expected .env to contain default value")
}

func TestSetupCmd_OutputFlag_DoesNotCreateStateFile(t *testing.T) {
	// Reset viper to prevent leaking config
	viperx.Reset()

	// Create a target DataRobot app directory
	targetAppDir := setupTestRepo(t)
	outputDir := t.TempDir()

	// Change to the app directory
	originalWd, _ := os.Getwd()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err := os.Chdir(targetAppDir)
	require.NoError(t, err)

	// Run setup with --output flag (simulating the behavior)
	err = setupNonInteractive(outputDir, filepath.Join(outputDir, ".env"))
	require.NoError(t, err, "setupNonInteractive should succeed")

	// Verify .env file was created in the output directory
	dotenvPath := filepath.Join(outputDir, ".env")

	_, err = os.Stat(dotenvPath)
	require.NoError(t, err, "Expected .env file to exist in output directory")

	// Verify state file was NOT created in the output directory (bug fix)
	statePath := filepath.Join(outputDir, ".datarobot", "cli", "state.yaml")

	_, err = os.Stat(statePath)
	require.Error(t, err, "State file should NOT be created when using --output flag")
	assert.True(t, os.IsNotExist(err), "State file should not exist in output directory")
}
