// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
}
