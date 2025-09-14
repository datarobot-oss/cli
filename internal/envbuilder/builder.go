// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"fmt"
	"maps"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

type UserPrompt struct {
	Key      string          `yaml:"key,omitempty"`
	Env      string          `yaml:"env"`
	Type     string          `yaml:"type"`
	Multiple bool            `yaml:"multiple"`
	Options  []PromptOptions `yaml:"options,omitempty"`
	Default  any             `yaml:"default,omitempty"`
	Help     string          `yaml:"help"`
	Optional bool            `yaml:"optional,omitempty"`
}

type PromptOptions struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value,omitempty"`
	Requires string `yaml:"requires,omitempty"`
}

type ParentOption struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type ParsedYaml map[string][]UserPrompt

func GatherUserPrompts(rootDir string) ([]UserPrompt, error) {
	yamlFiles, err := Discover(rootDir, 5)
	if err != nil {
		return nil, fmt.Errorf("failed to discover task yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return nil, nil
	}

	var allPrompts []UserPrompt

	for _, yamlFile := range yamlFiles {
		data, err := os.ReadFile(yamlFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read task yaml file %s: %w", yamlFile, err)
		}

		var fileParsed ParsedYaml

		if err = yaml.Unmarshal(data, &fileParsed); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task yaml file %s: %w", yamlFile, err)
		}

		allPrompts = append(allPrompts, promptsSorted(fileParsed, rootKeys(fileParsed))...)
	}

	return allPrompts, nil
}

func promptsSorted(fileParsed ParsedYaml, keys []string) []UserPrompt {
	sortedPrompts := make([]UserPrompt, 0)

	for _, key := range keys {
		for _, prompt := range fileParsed[key] {
			sortedPrompts = append(sortedPrompts, prompt)
			requiredPrompts := promptsSorted(fileParsed, requiredKeys(prompt))
			sortedPrompts = append(sortedPrompts, requiredPrompts...)
		}
	}

	return sortedPrompts
}

func rootKeys(fileParsed ParsedYaml) []string {
	keys := make(map[string]bool)

	for key := range maps.Keys(fileParsed) {
		keys[key] = true
	}

	for _, prompts := range fileParsed {
		for _, prompt := range prompts {
			for _, option := range prompt.Options {
				delete(keys, option.Requires)
			}
		}
	}

	return slices.Sorted(maps.Keys(keys))
}

func requiredKeys(prompt UserPrompt) []string {
	keys := make([]string, 0, len(prompt.Options))

	for _, option := range prompt.Options {
		if option.Requires != "" {
			keys = append(keys, option.Requires)
		}
	}

	return keys
}
