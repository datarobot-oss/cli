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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
  - env: REQUIRED_VAR
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

	t.Run("should not skip when core variables are set via environment but missing from .env", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte("root: []"), 0o644)
		require.NoError(t, err)

		// Create .env with template variables but no core DataRobot variables
		dotenvFile := filepath.Join(tmpDir, ".env")
		envContent := `INFRA_ENABLE_LLM=deployed_llm.py
LLM_DEPLOYMENT_ID=6a21895b92a225e438d83cc2
USE_DATAROBOT_LLM_GATEWAY=0
`
		err = os.WriteFile(dotenvFile, []byte(envContent), 0o644)
		require.NoError(t, err)

		// Core variables are set via environment, but should be ignored for skip decision
		t.Setenv("DATAROBOT_ENDPOINT", "https://app.datarobot.com/api/v2")
		t.Setenv("DATAROBOT_API_TOKEN", "test-token")

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.False(t, shouldSkip, "Should not skip when .env is missing core variables even if env vars provide them")
	})
}

// TestShouldSkipSetup_PresenceOnly covers the synthesis leaks that the file-only mode
// must not paper over: YAML defaults, auto-generated secrets, and commented-out entries
// must not satisfy the skip check. The wizard should run whenever the .env file itself
// does not contain an effective, uncommented value for every required variable.
func TestShouldSkipSetup_PresenceOnly(t *testing.T) {
	// helper to scaffold a repo with a parakeet.yaml and a .env, then assert skip.
	assertSkip := func(t *testing.T, parakeetYaml, envContent string, wantSkip bool, msg string) {
		t.Helper()

		tmpDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".datarobot", "cli"), 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".datarobot", "cli", "parakeet.yaml"), []byte(parakeetYaml), 0o644))

		dotenvFile := filepath.Join(tmpDir, ".env")
		require.NoError(t, os.WriteFile(dotenvFile, []byte(envContent), 0o644))

		shouldSkip, err := shouldSkipSetup(tmpDir, dotenvFile)
		require.NoError(t, err)
		require.Equal(t, wantSkip, shouldSkip, msg)
	}

	t.Run("does not skip when a required var is satisfied only by a YAML default", func(t *testing.T) {
		parakeetYaml := `root:
  - env: REQUIRED_WITH_DEFAULT
    default: some-default-value
    help: A required variable that has a default`
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
`
		assertSkip(t, parakeetYaml, envContent, false,
			"Should not skip when REQUIRED_WITH_DEFAULT is absent from .env even though it has a YAML default")
	})

	t.Run("does not skip when a required secret is satisfied only by generate:true", func(t *testing.T) {
		parakeetYaml := `root:
  - env: GENERATED_SECRET
    type: secret_string
    generate: true
    help: A required generated secret`
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
`
		assertSkip(t, parakeetYaml, envContent, false,
			"Should not skip when GENERATED_SECRET is absent from .env even though it would be auto-generated")
	})

	t.Run("does not skip when a required var is commented out in .env", func(t *testing.T) {
		parakeetYaml := `root:
  - env: REQUIRED_VAR
    help: A required variable`
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
# REQUIRED_VAR=commented-out-value
`
		assertSkip(t, parakeetYaml, envContent, false,
			"Should not skip when REQUIRED_VAR is commented out in .env")
	})

	t.Run("skips when a required var is present and uncommented in .env", func(t *testing.T) {
		parakeetYaml := `root:
  - env: REQUIRED_VAR
    help: A required variable`
		envContent := `DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2
DATAROBOT_API_TOKEN=test-token
REQUIRED_VAR=value
`
		assertSkip(t, parakeetYaml, envContent, true,
			"Should skip when all required vars are present and uncommented in .env")
	})
}
