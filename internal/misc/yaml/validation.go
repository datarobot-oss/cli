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
		return fmt.Errorf("failed to read file: %w", err)
	}

	var content interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("invalid YAML format: %w", err)
	}

	return nil
}
