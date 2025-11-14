// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package state

import (
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	stateFileName = "state.yaml"
	cliSubDir     = "cli"
	localStateDir = ".datarobot"
)

// State represents the current state of CLI interactions with a repository.
type State struct {
	// CLIVersion is the version of the CLI used for the successful run
	CLIVersion string `yaml:"cli_version"`
	// LastStart is an ISO8601-compliant timestamp of the last successful `dr start` run
	LastStart time.Time `yaml:"last_start"`
	// LastTemplatesSetup is an ISO8601-compliant timestamp of the last successful `dr templates setup` run
	LastTemplatesSetup *time.Time `yaml:"last_templates_setup,omitempty"`
	// LastDotenvSetup is an ISO8601-compliant timestamp of the last successful `dr dotenv setup` run
	LastDotenvSetup *time.Time `yaml:"last_dotenv_setup,omitempty"`
}

// GetStatePath determines the appropriate location for the state file.
// The state file is stored in .datarobot/cli directory within the current repository.
func GetStatePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Use local .datarobot/cli directory
	localPath := filepath.Join(cwd, localStateDir, cliSubDir)
	statePath := filepath.Join(localPath, stateFileName)

	return statePath, nil
}

// Load reads the state file from the appropriate location.
// Returns nil if the file doesn't exist (first run).
func Load() (*State, error) {
	statePath, err := GetStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist yet, not an error
		}

		return nil, err
	}

	var state State

	err = yaml.Unmarshal(data, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

// Update saves the state file and automatically sets the CLIVersion.
// This should be the preferred method for saving state.
func (s *State) Update() error {
	s.CLIVersion = version.Version

	return Save(s)
}

// Save writes the state file to the appropriate location.
// Creates parent directories if they don't exist.
// Note: Consider using Update() instead, which automatically sets CLIVersion.
func Save(state *State) error {
	statePath, err := GetStatePath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	stateDir := filepath.Dir(statePath)

	err = os.MkdirAll(stateDir, 0o755)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	err = os.WriteFile(statePath, data, 0o644)
	if err != nil {
		return err
	}

	return nil
}

// UpdateAfterSuccessfulRun creates or updates the state file after a successful `dr start` run.
func UpdateAfterSuccessfulRun() error {
	// Load existing state to preserve other fields
	existingState, err := Load()
	if err != nil {
		return err
	}

	if existingState == nil {
		existingState = &State{}
	}

	existingState.LastStart = time.Now().UTC()

	return existingState.Update()
}

// UpdateAfterDotenvSetup updates the state file after a successful `dr dotenv setup` run.
func UpdateAfterDotenvSetup() error {
	// Load existing state to preserve other fields
	existingState, err := Load()
	if err != nil {
		return err
	}

	if existingState == nil {
		existingState = &State{}
	}

	now := time.Now().UTC()
	existingState.LastDotenvSetup = &now

	return existingState.Update()
}

// UpdateAfterTemplatesSetup updates the state file after a successful `dr templates setup` run.
func UpdateAfterTemplatesSetup() error {
	// Load existing state to preserve other fields
	existingState, err := Load()
	if err != nil {
		return err
	}

	if existingState == nil {
		existingState = &State{}
	}

	now := time.Now().UTC()
	existingState.LastTemplatesSetup = &now

	return existingState.Update()
}

// HasCompletedDotenvSetup checks if dotenv setup has been completed in the past.
// If force_interactive flag is set, this always returns false to force re-execution.
func HasCompletedDotenvSetup() bool {
	// Check if we should force the wizard to run
	if viper.GetBool("force_interactive") {
		return false
	}

	state, err := Load()
	if err != nil || state == nil {
		return false
	}

	return state.LastDotenvSetup != nil && state.LastDotenvSetup.Before(time.Now())
}
