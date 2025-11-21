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
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/stretchr/testify/suite"
)

func TestTemplateTestSuite(t *testing.T) {
	suite.Run(t, new(TemplateTestSuite))
}

type TemplateTestSuite struct {
	suite.Suite
	tempDir string
	dotfile string
}

func (suite *TemplateTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-template-test")
	suite.tempDir = dir

	suite.dotfile = filepath.Join(suite.tempDir, ".env")

	suite.T().Setenv("DATAROBOT_ENDPOINT", "")
	suite.T().Setenv("DATAROBOT_ENDPOINT_SHORT", "")
	suite.T().Setenv("DATAROBOT_API_TOKEN", "")
}

func (suite *TemplateTestSuite) TestCreateDotenvWithoutTemplate() {
	_, contents, err := updateDotenvFile(suite.dotfile)

	suite.Require().NoError(err)

	suite.FileExists(suite.dotfile, "Expected dotenv file to be created")

	suite.Regexp(`(?m:^DATAROBOT_ENDPOINT=""$)`, contents)
	suite.Regexp(`(?m:^DATAROBOT_API_TOKEN=""$)`, contents)

	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Verify header format with timestamp
	suite.Regexp(`# Edited using .dr dotenv. on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)
	suite.Regexp(`(?m:^DATAROBOT_ENDPOINT=""$)`, content)
	suite.Regexp(`(?m:^DATAROBOT_API_TOKEN=""$)`, content)

	os.Remove(suite.dotfile)
}

func (suite *TemplateTestSuite) TestCreateDotenvFromScratch() {
	variables, contents, err := updateDotenvFile(suite.dotfile)

	suite.Require().NoError(err)

	suite.FileExists(suite.dotfile, "Expected dotenv file to be created")

	suite.Equal(
		[]envbuilder.Variable{
			{Name: "DATAROBOT_ENDPOINT", Value: "", Secret: false, Commented: false},
			{Name: "DATAROBOT_API_TOKEN", Value: "", Secret: true, Commented: false},
		},
		variables,
	)
	suite.Regexp(`(?m:^DATAROBOT_ENDPOINT=""$)`, contents)
	suite.Regexp(`(?m:^DATAROBOT_API_TOKEN=""$)`, contents)

	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Verify header format with timestamp
	suite.Regexp(`# Edited using .dr dotenv. on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)
	suite.Regexp(`(?m:^DATAROBOT_ENDPOINT=""$)`, content)
	suite.Regexp(`(?m:^DATAROBOT_API_TOKEN=""$)`, content)

	os.Remove(suite.dotfile)
}

func (suite *TemplateTestSuite) TestReadDotfile() {
	_ = os.WriteFile(suite.dotfile, []byte("DATAROBOT_ENDPOINT=\n"), 0o644)

	suite.FileExists(suite.dotfile, "Expected dotenv file to be created")

	dotenvFileLines, _ := readDotenvFile(suite.dotfile)

	suite.Equal(
		[]string{"DATAROBOT_ENDPOINT=\n"},
		dotenvFileLines,
	)

	os.Remove(suite.dotfile)
}

func (suite *TemplateTestSuite) TestMultipleSavesDoNotDuplicateHeader() {
	_ = os.WriteFile(suite.dotfile, []byte("DATAROBOT_ENDPOINT=\"\"\n"), 0o644)

	suite.FileExists(suite.dotfile, "Expected dotenv template file to be created")

	// First save
	_, _, err := updateDotenvFile(suite.dotfile)
	suite.Require().NoError(err)

	// Second save (simulating user editing and saving again)
	_, _, err = updateDotenvFile(suite.dotfile)
	suite.Require().NoError(err)

	// Read the file and count how many times the header appears
	dotfileContents, _ := os.ReadFile(suite.dotfile)
	content := string(dotfileContents)

	// Count header comment lines (lines starting with #)
	lines := strings.Split(content, "\n")
	headerCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# Edited using") {
			headerCount++
		}
	}

	// Should only have one header comment
	suite.Equal(1, headerCount, "Expected only one header comment, but found %d", headerCount)

	// Verify the header format with timestamp
	suite.Contains(content, "# Edited using `dr dotenv`")
	suite.Regexp(`# Edited using .dr dotenv. on \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, content)

	os.Remove(suite.dotfile)
}

func (suite *TemplateTestSuite) TestGetStateDir() {
	// Test with XDG_STATE_HOME set
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)

	stateDir, err := getStateDir()
	suite.Require().NoError(err)
	suite.Equal(filepath.Join(xdgStateHome, "dr", "backups"), stateDir)
	suite.DirExists(stateDir)

	// Test without XDG_STATE_HOME (should use default)
	suite.T().Setenv("XDG_STATE_HOME", "")

	stateDir, err = getStateDir()
	suite.Require().NoError(err)
	suite.Contains(stateDir, filepath.Join(".local", "state", "dr", "backups"))
}

func (suite *TemplateTestSuite) TestGetBackupBaseName() {
	testFile := filepath.Join(suite.tempDir, ".env")
	baseName := getBackupBaseName(testFile)

	// Should contain the filename
	suite.Contains(baseName, ".env")

	// Should contain an underscore separator
	suite.Contains(baseName, "_")

	// Should have the format: filename_hash
	parts := strings.Split(baseName, "_")
	suite.Len(parts, 2)
	suite.Equal(".env", parts[0])
	suite.Len(parts[1], 8) // 8-character hash

	// Same file should produce same base name
	baseName2 := getBackupBaseName(testFile)
	suite.Equal(baseName, baseName2)

	// Different file should produce different base name
	differentFile := filepath.Join(suite.tempDir, "subdir", ".env")
	differentBaseName := getBackupBaseName(differentFile)
	suite.NotEqual(baseName, differentBaseName)
}

func (suite *TemplateTestSuite) TestBackupCreatesFile() {
	// Create a test file to backup
	testContent := "DATAROBOT_ENDPOINT=test\n"
	err := os.WriteFile(suite.dotfile, []byte(testContent), 0o644)
	suite.Require().NoError(err)

	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)

	// Perform backup
	err = backup(suite.dotfile)
	suite.Require().NoError(err)

	// Verify backup was created
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")
	suite.DirExists(stateDir)

	// Find backup files
	baseName := getBackupBaseName(suite.dotfile)
	pattern := filepath.Join(stateDir, baseName+"_*")
	matches, err := filepath.Glob(pattern)
	suite.Require().NoError(err)
	suite.Len(matches, 1, "Expected exactly one backup file")

	// Verify backup content
	backupContent, err := os.ReadFile(matches[0])
	suite.Require().NoError(err)
	suite.Equal(testContent, string(backupContent))

	// Verify original file still exists
	suite.FileExists(suite.dotfile)
	originalContent, err := os.ReadFile(suite.dotfile)
	suite.Require().NoError(err)
	suite.Equal(testContent, string(originalContent))
}

func (suite *TemplateTestSuite) TestBackupNonExistentFile() {
	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)

	// Try to backup a file that doesn't exist
	nonExistentFile := filepath.Join(suite.tempDir, "nonexistent.env")
	err := backup(nonExistentFile)
	suite.Require().NoError(err, "Backing up non-existent file should not error")

	// Verify no backup was created
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")
	if _, err := os.Stat(stateDir); err == nil {
		baseName := getBackupBaseName(nonExistentFile)
		pattern := filepath.Join(stateDir, baseName+"_*")
		matches, err := filepath.Glob(pattern)
		suite.Require().NoError(err)
		suite.Empty(matches, "No backup should be created for non-existent file")
	}
}

func (suite *TemplateTestSuite) TestCleanOldBackups() {
	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")
	err := os.MkdirAll(stateDir, 0o755)
	suite.Require().NoError(err)

	baseName := getBackupBaseName(suite.dotfile)

	// Create 5 backup files with slight time delays to ensure different mtimes
	for i := 0; i < 5; i++ {
		timestamp := time.Now().Add(-time.Duration(5-i) * time.Hour).Format("2006-01-02_15-04-05")
		backupFileName := filepath.Join(stateDir, baseName+"_"+timestamp)
		err := os.WriteFile(backupFileName, []byte("content"+string(rune(i))), 0o644)
		suite.Require().NoError(err)

		// Small delay to ensure different modification times
		time.Sleep(10 * time.Millisecond)
	}

	// Verify we have 5 backups
	pattern := filepath.Join(stateDir, baseName+"_*")
	matches, err := filepath.Glob(pattern)
	suite.Require().NoError(err)
	suite.Len(matches, 5)

	// Clean old backups
	err = cleanOldBackups(stateDir, baseName)
	suite.Require().NoError(err)

	// Should only have 3 remaining
	matches, err = filepath.Glob(pattern)
	suite.Require().NoError(err)
	suite.Len(matches, 3, "Should keep only 3 most recent backups")
}

func (suite *TemplateTestSuite) TestCleanOldBackupsKeepsThreeOrLess() {
	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")
	err := os.MkdirAll(stateDir, 0o755)
	suite.Require().NoError(err)

	baseName := getBackupBaseName(suite.dotfile)

	// Create only 2 backup files
	for i := 0; i < 2; i++ {
		timestamp := time.Now().Add(-time.Duration(2-i) * time.Hour).Format("2006-01-02_15-04-05")
		backupFileName := filepath.Join(stateDir, baseName+"_"+timestamp)
		err := os.WriteFile(backupFileName, []byte("content"), 0o644)
		suite.Require().NoError(err)
	}

	// Clean old backups
	err = cleanOldBackups(stateDir, baseName)
	suite.Require().NoError(err)

	// Should still have 2 (no deletion)
	pattern := filepath.Join(stateDir, baseName+"_*")
	matches, err := filepath.Glob(pattern)
	suite.Require().NoError(err)
	suite.Len(matches, 2, "Should not delete when 3 or fewer backups exist")
}

func (suite *TemplateTestSuite) TestMultipleBackupsIntegration() {
	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")

	// Create initial file
	err := os.WriteFile(suite.dotfile, []byte("version1"), 0o644)
	suite.Require().NoError(err)

	// Create 4 backups with delays to ensure unique timestamps
	for i := 0; i < 4; i++ {
		// Sleep for over a second to ensure different timestamps
		time.Sleep(1100 * time.Millisecond)

		err = backup(suite.dotfile)
		suite.Require().NoError(err)

		// Update file content
		err = os.WriteFile(suite.dotfile, []byte("version"+string(rune(i+2))), 0o644)
		suite.Require().NoError(err)
	}

	// Should only have 3 backups due to automatic cleanup
	baseName := getBackupBaseName(suite.dotfile)
	pattern := filepath.Join(stateDir, baseName+"_*")
	matches, err := filepath.Glob(pattern)
	suite.Require().NoError(err)
	suite.Len(matches, 3, "Should maintain only 3 backups after cleanup")
}

func (suite *TemplateTestSuite) TestBackupDifferentFiles() {
	// Set up a temporary XDG_STATE_HOME
	xdgStateHome := filepath.Join(suite.tempDir, "xdg_state")
	suite.T().Setenv("XDG_STATE_HOME", xdgStateHome)

	// Create two different .env files in different directories
	dir1 := filepath.Join(suite.tempDir, "project1")
	dir2 := filepath.Join(suite.tempDir, "project2")
	err := os.MkdirAll(dir1, 0o755)
	suite.Require().NoError(err)
	err = os.MkdirAll(dir2, 0o755)
	suite.Require().NoError(err)

	file1 := filepath.Join(dir1, ".env")
	file2 := filepath.Join(dir2, ".env")

	err = os.WriteFile(file1, []byte("project1 content"), 0o644)
	suite.Require().NoError(err)
	err = os.WriteFile(file2, []byte("project2 content"), 0o644)
	suite.Require().NoError(err)

	// Backup both files
	err = backup(file1)
	suite.Require().NoError(err)
	err = backup(file2)
	suite.Require().NoError(err)

	// Verify both backups exist with different names
	stateDir := filepath.Join(xdgStateHome, "dr", "backups")
	allBackups, err := filepath.Glob(filepath.Join(stateDir, "*"))
	suite.Require().NoError(err)
	suite.Len(allBackups, 2, "Should have backups for both files")

	// Verify the base names are different
	baseName1 := getBackupBaseName(file1)
	baseName2 := getBackupBaseName(file2)
	suite.NotEqual(baseName1, baseName2, "Different paths should have different base names")
}
