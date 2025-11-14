// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

var (
	configFileDir  = filepath.Join(".config", "datarobot") // Can we also support XDG_CONFIG_HOME?
	configFileName = "drconfig.yaml"
)

func CreateConfigFileDirIfNotExists() error {
	// Set the default config file directory here to aid in testing
	defaultConfigFileDir := filepath.Join(os.Getenv("HOME"), configFileDir)
	defaultConfigFilePath := filepath.Join(defaultConfigFileDir, configFileName)

	_, err := os.Stat(defaultConfigFilePath)
	if err == nil {
		// File exists, do nothing
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error checking config file: %w", err)
	}

	// file was not found, let's create it

	err = os.MkdirAll(defaultConfigFileDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create config file directory: %w", err)
	}

	_, err = os.Create(defaultConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	return nil
}

func ReadConfigFile(filePath string) error {
	// Set the default config file directory here to aid in testing
	defaultConfigFileDir := filepath.Join(os.Getenv("HOME"), configFileDir)

	viper.SetConfigType("yaml")

	if filePath != "" {
		if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
			return fmt.Errorf("config file must have .yaml or .yml extension: %s", filePath)
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
			return fmt.Errorf("failed to generate debug config output: %w", err)
		}

		fmt.Print(output)
	}

	return nil
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
	// TODO There has to be a better way of marking sensitive data
	// perhaps with leebenson/conform?
	keys := make([]string, 0, len(viper.AllSettings()))
	for key := range viper.AllSettings() {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := viper.Get(key)

		// TODO Come up with a better way of redacting sensitive information
		if key == "token" || key == "api_token" {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, "****"))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	return sb.String(), nil
}
