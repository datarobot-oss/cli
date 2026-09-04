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

// configFileMode is the permission drconfig.yaml must have: it stores the API
// token in plaintext, so it is owner-only.
const configFileMode = 0o600

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

	// Create with 0600 rather than os.Create's 0666&^umask: this file holds the API
	// token. os.WriteFile in UpdateConfigFile cannot fix the mode later, because its
	// perm argument only applies when it creates the file itself.
	f, err := os.OpenFile(defaultConfigFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, configFileMode)
	if err != nil {
		return fmt.Errorf("Failed to create config file: %w", err)
	}

	// Close the handle immediately. On Windows an open handle blocks the file
	// from being removed (e.g. t.TempDir cleanup); POSIX silently tolerates it.
	if err := f.Close(); err != nil {
		return fmt.Errorf("Failed to close config file: %w", err)
	}

	return nil
}

func ReadConfigFile(filePath string) error {
	defaultConfigFileDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	if err := configureViperSearchPath(filePath, defaultConfigFileDir); err != nil {
		return err
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

	if name := ActiveProfile(); name != "" {
		if err := applyProfile(name); err != nil {
			return err
		}
	}

	return printDebugConfigIfEnabled()
}

// configureViperSearchPath points viper at filePath, or at
// defaultConfigFileDir/configFileName when filePath is empty.
func configureViperSearchPath(filePath, defaultConfigFileDir string) error {
	viper.SetConfigType("yaml")

	if filePath == "" {
		viper.SetConfigName(configFileName)
		viper.AddConfigPath(defaultConfigFileDir)

		return nil
	}

	if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
		return fmt.Errorf("Config file must have .yaml or .yml extension: %s.", filePath)
	}

	viper.SetConfigName(filepath.Base(filePath))
	viper.AddConfigPath(filepath.Dir(filePath))

	return nil
}

// printDebugConfigIfEnabled prints the effective viper configuration when
// --debug is set.
func printDebugConfigIfEnabled() error {
	if !viper.GetBool("debug") {
		return nil
	}

	output, err := DebugViperConfig()
	if err != nil {
		return fmt.Errorf("Failed to generate debug config output: %w", err)
	}

	fmt.Print(output)

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
	sb.WriteString("\n")

	activeProfile := ActiveProfile()
	if activeProfile == "" {
		activeProfile = "default"
	}

	sb.WriteString("Active profile: ")
	sb.WriteString(activeProfile)
	sb.WriteString("\n\n")

	// Print out the viper configuration for debugging, alphabetically, and
	// redacting sensitive information at any nesting depth: a named
	// profile's endpoint/token/etc. live under "profiles.<name>.*", below
	// the top level, so a top-level-only redaction check would print a
	// non-default profile's token in the clear.
	settings := redactSettings(viper.AllSettings())

	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		fmt.Fprintf(&sb, "  %s: %v\n", key, settings[key])
	}

	return sb.String(), nil
}

// redactSettings returns a deep copy of m with the value of every key in
// sensitiveDebugKeys replaced by "****", at any nesting depth. Nested named
// profiles put credentials below the top level, so a shallow check would
// print them in the clear.
func redactSettings(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))

	for key, value := range m {
		if _, sensitive := sensitiveDebugKeys[key]; sensitive {
			out[key] = "****"
			continue
		}

		out[key] = redactValue(value)
	}

	return out
}

// redactValue recurses into nested maps and slices so redactSettings can
// redact sensitive keys regardless of depth. Viper's YAML decoding path can
// produce either map[string]any or map[any]any for nested maps.
func redactValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return redactSettings(v)

	case map[any]any:
		asStringMap, _ := toStringKeyedMap(v)

		return redactSettings(asStringMap)

	case []any:
		redacted := make([]any, len(v))

		for i, item := range v {
			redacted[i] = redactValue(item)
		}

		return redacted

	default:
		return value
	}
}

// toStringKeyedMap normalizes a value that is either map[string]any or
// map[any]any into map[string]any, or reports false for anything else.
// Viper's YAML decoding path can produce either shape for a nested map
// depending on how deeply it is nested, so both redactValue and profile.go's
// normalizeSectionMap need this same conversion.
func toStringKeyedMap(v any) (map[string]any, bool) {
	if asMap, ok := v.(map[string]any); ok {
		return asMap, true
	}

	rawMap, ok := v.(map[any]any)
	if !ok {
		return nil, false
	}

	normalized := make(map[string]any, len(rawMap))

	for key, val := range rawMap {
		normalized[fmt.Sprintf("%v", key)] = val
	}

	return normalized, true
}
