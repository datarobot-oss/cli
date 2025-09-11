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
	"os"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
)

type UserPrompt struct {
	Key     string
	Env     string `yaml:"env"`
	Type    string `yaml:"type"`
	Options []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value,omitempty"`
	} `yaml:"options,omitempty"`
	Default  string `yaml:"default,omitempty"`
	Help     string `yaml:"help"`
	Optional bool   `yaml:"optional,omitempty"`
	Requires []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"requires,omitempty"`
}

type UserPromptCollection struct {
	Key      string
	Requires []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"requires,omitempty"`
	Prompts []UserPrompt `yaml:"prompts"`
}

type BuilderOpts struct {
	BinaryName string
	Dir        string
	Stdout     *os.File
	Stderr     *os.File
	Stdin      *os.File
}

type Builder struct{}

func NewEnvBuilder() *Builder {

	return &Builder{}
}

func (r *Builder) GatherUserPrompts(rootDir string) ([]interface{}, error) {

	yamlFiles, err := Discover(rootDir, 2)
	if err != nil {
		return nil, fmt.Errorf("failed to discover task yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return nil, nil
	}

	var fullCollection []interface{}

	for _, f := range yamlFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read task yaml file %s: %w", f, err)
		}
		var collection map[string]interface{}
		if err = yaml.Unmarshal(data, &collection); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task yaml file %s: %w", f, err)
		}

		for key, value := range collection {
			itemMap, ok := value.(map[interface{}]interface{})
			if !ok {
				return nil, fmt.Errorf("unexpected format in yaml file %s for key %s", f, key)
			}
			if prompts, exists := itemMap["prompts"]; exists {
				// This is a UserPromptCollection
				var userPromptCollection UserPromptCollection
				userPromptCollection.Key = key
				var promptsList []UserPrompt
				promptsSlice, ok := prompts.([]interface{})
				if !ok {
					return nil, fmt.Errorf("unexpected format for prompts in yaml file %s for key %s", f, key)
				}
				for _, p := range promptsSlice {
					pMap, ok := p.(map[interface{}]interface{})
					if !ok {
						return nil, fmt.Errorf("unexpected format for individual prompt in yaml file %s for key %s", f, key)
					}
					var userPrompt UserPrompt
					err = mapstructure.Decode(pMap, &userPrompt)
					if err != nil {
						return nil, fmt.Errorf("failed to decode individual prompt in yaml file %s for key %s: %w", f, key, err)
					}
					promptsList = append(promptsList, userPrompt)
				}
				userPromptCollection.Prompts = promptsList
				delete(itemMap, "prompts")
				err = mapstructure.Decode(itemMap, &userPromptCollection)
				if err != nil {
					return nil, fmt.Errorf("failed to decode prompts collection in yaml file %s: %w", f, err)
				}
				fullCollection = append(fullCollection, userPromptCollection)
			} else {
				// This is a map of UserPrompt
				var userPrompt UserPrompt
				userPrompt.Key = key
				err = mapstructure.Decode(itemMap, &userPrompt)
				if err != nil {
					return nil, fmt.Errorf("failed to decode prompt in yaml file %s: %w", f, err)
				}
				fullCollection = append(fullCollection, userPrompt)
			}
		}
	}

	return fullCollection, nil
}
