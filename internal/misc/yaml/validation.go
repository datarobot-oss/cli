// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package yaml

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// IsValidYAML checks if the file at the given path is a valid YAML file.
// It does not care if we are using .yml or .yaml extension.
func IsValidYAML(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Failed to read file: %w", err)
	}

	var content interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("Invalid YAML format: %w", err)
	}

	return nil
}
