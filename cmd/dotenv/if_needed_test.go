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

package dotenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Clear DataRobot environment variables that might leak from the host environment
	// into tests, ensuring tests have a clean isolated environment.
	os.Unsetenv("DATAROBOT_ENDPOINT")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	os.Exit(m.Run())
}

func TestShouldSkipSetup(t *testing.T) {
	t.Run("should skip when .env exists and validation passes", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		// Create parakeet.yaml with no required variables
		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		// Create .env with core DataRobot variables that are always required
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
EXISTING_VAR=value
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.True(t, shouldSkip, "Should skip setup when .env exists and validation passes")
	})

	t.Run("should not skip when .env does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		dotenvFile := filepath.Join(tmpDir, ".env")

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip setup when .env does not exist")
	})

	t.Run("should not skip when validation fails", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		// Create parakeet.yaml with a required variable
		parakeetYaml := `root:
  - field: REQUIRED_VAR
    help: A required variable for testing`
		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte(parakeetYaml), 0o644)
		require.NoError(t, err)

		// Create .env with core DataRobot variables but missing the custom required variable
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
OTHER_VAR=value
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip setup when validation fails")
	})

	t.Run("should skip when no parakeet.yaml exists but core variables are set", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		// Don't create parakeet.yaml - only core variables will be checked

		// Create .env with core DataRobot variables
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.True(t, shouldSkip, "Should skip when core variables are set even without parakeet.yaml")
	})

	t.Run("should not skip when core variables are missing", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		// Create .env without core DataRobot variables
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `SOME_OTHER_VAR=value
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip when core DataRobot variables are missing")
	})

	t.Run("should not skip when only DATAROBOT_ENDPOINT is set", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		// Create .env with only DATAROBOT_ENDPOINT (missing DATAROBOT_API_TOKEN)
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip when DATAROBOT_API_TOKEN is missing")
	})

	t.Run("should not skip when only DATAROBOT_API_TOKEN is set", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		// Create .env with only DATAROBOT_API_TOKEN (missing DATAROBOT_ENDPOINT)
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `DATAROBOT_API_TOKEN=test-token
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip when DATAROBOT_ENDPOINT is missing")
	})
}
