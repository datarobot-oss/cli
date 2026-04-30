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
