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

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

var configFileName = "drconfig.yaml"

// GetConfigDir returns the config directory, respecting XDG_CONFIG_HOME if set.
// Falls back to ~/.config/datarobot if XDG_CONFIG_HOME is not set.
func GetConfigDir() (string, error) {
	configHome, err := resolveConfigHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(configHome, "datarobot"), nil
}

// resolveConfigHome returns the XDG config home directory. It uses the value
// parsed by github.com/adrg/xdg when XDG_CONFIG_HOME is explicitly set;
// otherwise it falls back to ~/.config regardless of OS.
func resolveConfigHome() (string, error) {
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		return xdg.ConfigHome, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config"), nil
}

// GetConfigDirs returns additional XDG config search directories from
// XDG_CONFIG_DIRS, in priority order, parsed via github.com/adrg/xdg. It
// returns nil when the environment variable is not explicitly set: searching
// extra system directories is opt-in only, no OS-specific defaults are used.
func GetConfigDirs() []string {
	if os.Getenv("XDG_CONFIG_DIRS") == "" {
		return nil
	}

	return xdg.ConfigDirs
}

// GetStateDir returns the XDG state directory, respecting XDG_STATE_HOME if
// set. Falls back to ~/.local/state if not set, regardless of OS (mirroring
// GetConfigDir's fallback behavior).
func GetStateDir() (string, error) {
	if os.Getenv("XDG_STATE_HOME") != "" {
		return xdg.StateHome, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".local", "state"), nil
}

func CreateConfigFileDirIfNotExists() error {
	defaultConfigFileDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	defaultConfigFilePath := filepath.Join(defaultConfigFileDir, configFileName)

	_, err = os.Stat(defaultConfigFilePath)
	if err == nil {
		// File exists, do nothing
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Error checking config file: %w", err)
	}

	// file was not found, let's create it

	err = os.MkdirAll(defaultConfigFileDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Failed to create config file directory: %w", err)
	}

	_, err = os.Create(defaultConfigFilePath)
	if err != nil {
		return fmt.Errorf("Failed to create config file: %w", err)
	}

	return nil
}

func ReadConfigFile(filePath string) error {
	defaultConfigFileDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	viper.SetConfigType("yaml")

	if filePath != "" {
		if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
			return fmt.Errorf("Config file must have .yaml or .yml extension: %s.", filePath)
		}

		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)
		viper.SetConfigName(filename)
		viper.AddConfigPath(dir)
	} else {
		viper.SetConfigName(configFileName)
		viper.AddConfigPath(defaultConfigFileDir)
	}

	// Read in the config file
	// Ignore error if config file not found, because that's fine
	// but return on all other errors
	if err := viper.ReadInConfig(); err != nil {
		// The zero-value struct looks weird, but we are using
		// errors.As which only does type checking. We don't
		// need an actual instance of the error.
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) {
			return err
		}
	}

	if viper.GetBool("debug") {
		output, err := DebugViperConfig()
		if err != nil {
			return fmt.Errorf("Failed to generate debug config output: %w", err)
		}

		fmt.Print(output)
	}

	return nil
}

// This is a list of keys that we want to redact
// when printing out the viper configuration for
// debugging. This is not a comprehensive list,
// but it should cover the most common cases.
// TODO There has to be a better way of marking sensitive data
// perhaps with leebenson/conform?
var sensitiveDebugKeys = map[string]struct{}{
	"token":                    {},
	"pulumi_config_passphrase": {},
}

func DebugViperConfig() (string, error) {
	var sb strings.Builder

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		configFile = "none (using defaults and environment variables)"
	}

	sb.WriteString("Configuration initialized. Using config file: ")
	sb.WriteString(configFile)
	sb.WriteString("\n\n")

	// Print out the viper configuration for debugging
	// Alphabetically, and redacting sensitive information
	keys := make([]string, 0, len(viper.AllSettings()))
	for key := range viper.AllSettings() {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := viper.Get(key)

		// Redact sensitive keys
		if _, sensitive := sensitiveDebugKeys[key]; sensitive {
			fmt.Fprintf(&sb, "  %s: %s\n", key, "****")
		} else {
			fmt.Fprintf(&sb, "  %s: %v\n", key, value)
		}
	}

	return sb.String(), nil
}
