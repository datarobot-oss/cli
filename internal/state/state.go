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

	"gopkg.in/yaml.v3"
)

const (
	stateFileName = "currentstate.yml"
	stateSubDir   = "state"
	localStateDir = ".datarobot"
)

// State represents the current state of CLI interactions with a repository.
type State struct {
	// LastSuccessfulRun is an ISO8601-compliant timestamp of the last successful `dr start` run
	LastSuccessfulRun time.Time `yaml:"last_successful_run"`
	// CLIVersion is the version of the CLI used for the successful run
	CLIVersion string `yaml:"cli_version"`
	// LastDotenvSetup is an ISO8601-compliant timestamp of the last successful `dr dotenv setup` run
	LastDotenvSetup *time.Time `yaml:"last_dotenv_setup,omitempty"`
}

// GetStatePath determines the appropriate location for the state file.
// The state file is stored in .datarobot/state directory within the current repository.
func GetStatePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Use local .datarobot/state directory
	localPath := filepath.Join(cwd, localStateDir, stateSubDir)
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

// Save writes the state file to the appropriate location.
// Creates parent directories if they don't exist.
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
func UpdateAfterSuccessfulRun(cliVersion string) error {
	// Load existing state to preserve other fields
	existingState, err := Load()
	if err != nil {
		return err
	}

	if existingState == nil {
		existingState = &State{}
	}

	existingState.LastSuccessfulRun = time.Now().UTC()
	existingState.CLIVersion = cliVersion

	return Save(existingState)
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

	return Save(existingState)
}

// HasCompletedDotenvSetup checks if dotenv setup has been completed in the past.
func HasCompletedDotenvSetup() bool {
	state, err := Load()
	if err != nil || state == nil {
		return false
	}

	return state.LastDotenvSetup != nil && state.LastDotenvSetup.Before(time.Now())
}
