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

package state

import (
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var stateFilePath = filepath.Join(".datarobot", "cli", "state.yaml")

// state represents the current state of CLI interactions with a repository.
type state struct {
	// CLIVersion is the version of the CLI used for the successful run
	CLIVersion string `yaml:"cli_version"`
	// LastStart is an ISO8601-compliant timestamp of the last successful `dr start` run
	LastStart time.Time `yaml:"last_start"`
	// LastTemplatesSetup is an ISO8601-compliant timestamp of the last successful `dr templates setup` run
	LastTemplatesSetup *time.Time `yaml:"last_templates_setup,omitempty"`
	// LastDotenvSetup is an ISO8601-compliant timestamp of the last successful `dr dotenv setup` run
	LastDotenvSetup *time.Time `yaml:"last_dotenv_setup,omitempty"`
}

// getStatePath determines the appropriate location for the state file.
// The state file is stored in .datarobot/cli directory within the current repository.
func getStatePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Use local .datarobot/cli directory
	return filepath.Join(cwd, stateFilePath), nil
}

// load reads the state file from the appropriate location.
// Returns nil if the file doesn't exist (first run).
func load() (state, error) {
	statePath, err := getStatePath()
	if err != nil {
		return state{}, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state{}, nil // File doesn't exist yet, not an error
		}

		return state{}, err
	}

	var existingState state

	err = yaml.Unmarshal(data, &existingState)
	if err != nil {
		return state{}, err
	}

	return existingState, nil
}

// update saves the state file and automatically sets the CLIVersion.
// This should be the preferred method for saving state.
func (s state) update() error {
	s.CLIVersion = version.Version

	return s.save()
}

// save writes the state file to the appropriate location.
// Creates parent directories if they don't exist.
// Note: Consider using update() instead, which automatically sets CLIVersion.
func (s state) save() error {
	statePath, err := getStatePath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	stateDir := filepath.Dir(statePath)

	err = os.MkdirAll(stateDir, 0o755)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(s)
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
	existingState, err := load()
	if err != nil {
		return err
	}

	existingState.LastStart = time.Now().UTC()

	return existingState.update()
}

// UpdateAfterDotenvSetup updates the state file after a successful `dr dotenv setup` run.
func UpdateAfterDotenvSetup() error {
	// Load existing state to preserve other fields
	existingState, err := load()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	existingState.LastDotenvSetup = &now

	return existingState.update()
}

// UpdateAfterTemplatesSetup updates the state file after a successful `dr templates setup` run.
func UpdateAfterTemplatesSetup() error {
	// Load existing state to preserve other fields
	existingState, err := load()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	existingState.LastTemplatesSetup = &now

	return existingState.update()
}

// HasCompletedDotenvSetup checks if dotenv setup has been completed in the past.
// If force-interactive flag is set, this always returns false to force re-execution.
func HasCompletedDotenvSetup() bool {
	// Check if we should force the wizard to run
	if viper.GetBool("force-interactive") {
		return false
	}

	existingState, err := load()
	if err != nil {
		return false
	}

	return existingState.LastDotenvSetup != nil &&
		existingState.LastDotenvSetup.Before(time.Now())
}
