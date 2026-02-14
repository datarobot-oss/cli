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
