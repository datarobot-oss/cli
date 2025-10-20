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

	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
)

type UserPrompt struct {
	Section  string
	Env      string         `yaml:"env"`
	Key      string         `yaml:"key"`
	Type     string         `yaml:"type"`
	Multiple bool           `yaml:"multiple"`
	Options  []PromptOption `yaml:"options,omitempty"`
	Default  any            `yaml:"default,omitempty"`
	Help     string         `yaml:"help"`
	Optional bool           `yaml:"optional,omitempty"`
}

type PromptOption struct {
	Blank    bool
	Checked  bool
	Name     string `yaml:"name"`
	Value    string `yaml:"value,omitempty"`
	Requires string `yaml:"requires,omitempty"`
}

type ParsedYaml map[string][]UserPrompt

func GatherUserPrompts(rootDir string) ([]UserPrompt, []string, error) {
	yamlFiles, err := Discover(rootDir, 5)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to discover task yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return nil, nil, nil
	}

	allPrompts := make([]UserPrompt, 0)
	allRootKeys := make([]string, 0)

	for _, yamlFile := range yamlFiles {
		prompts, roots, err := filePrompts(yamlFile)
		if err != nil {
			log.Debug(err)
			continue
		}

		allPrompts = append(allPrompts, prompts...)
		allRootKeys = append(allRootKeys, roots...)
	}

	return allPrompts, allRootKeys, nil
}

func filePrompts(yamlFile string) ([]UserPrompt, []string, error) {
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read task yaml file %s: %w", yamlFile, err)
	}

	var fileParsed ParsedYaml

	if err = yaml.Unmarshal(data, &fileParsed); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal task yaml file %s: %w", yamlFile, err)
	}

	roots := rootSections(fileParsed)
	prompts := promptsSorted(fileParsed, yamlFile, roots)

	for i, root := range roots {
		roots[i] = yamlFile + ":" + root
	}

	for p := range prompts {
		if prompts[p].Key != "" {
			prompts[p].Env = "# " + prompts[p].Key
		}

		prompts[p].Section = yamlFile + ":" + prompts[p].Section

		for o := range prompts[p].Options {
			if prompts[p].Options[o].Requires != "" {
				prompts[p].Options[o].Requires = yamlFile + ":" + prompts[p].Options[o].Requires
			}

			if prompts[p].Options[o].Value == "" {
				prompts[p].Options[o].Value = prompts[p].Options[o].Name
			}
		}
	}

	return prompts, roots, nil
}

func promptsSorted(fileParsed ParsedYaml, yamlFile string, keys []string) []UserPrompt {
	sortedPrompts := make([]UserPrompt, 0)

	for _, key := range keys {
		for _, prompt := range fileParsed[key] {
			prompt.Section = key

			sortedPrompts = append(sortedPrompts, prompt)

			requiredPrompts := promptsSorted(fileParsed, yamlFile, requiredSections(prompt))
			sortedPrompts = append(sortedPrompts, requiredPrompts...)
		}
	}

	return sortedPrompts
}

func rootSections(fileParsed ParsedYaml) []string {
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

func requiredSections(prompt UserPrompt) []string {
	keys := make([]string, 0, len(prompt.Options))

	for _, option := range prompt.Options {
		if option.Requires != "" {
			keys = append(keys, option.Requires)
		}
	}

	return keys
}
