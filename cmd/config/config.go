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
	"strings"

	"github.com/spf13/viper"
)

var (
	configFileDir  = "/.config/datarobot"
	configFileName = "drconfig.yaml"
)

func CreateConfigFileDirIfNotExists() error {
	// Set the default config file directory here to aid in testing
	defaultConfigFileDir := os.Getenv("HOME") + configFileDir
	defaultConfigFilePath := defaultConfigFileDir + "/" + configFileName

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
	defaultConfigFileDir := os.Getenv("HOME") + configFileDir

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
		if err := CreateConfigFileDirIfNotExists(); err != nil {
			panic(err)
		}

		viper.SetConfigName(configFileName)
		viper.AddConfigPath(defaultConfigFileDir)
	}

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	return nil
}
