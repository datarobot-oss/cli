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
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
)

type PromptType string

const (
	PromptTypeString PromptType = "string"
	PromptTypeSecret PromptType = "secret_string"
)

func (pt PromptType) String() string {
	return string(pt)
}

type UserPrompt struct {
	Section   string
	Root      bool
	Active    bool
	Commented bool
	Value     string
	Hidden    bool

	Env      string         `yaml:"env"`
	Key      string         `yaml:"key"`
	Type     PromptType     `yaml:"type"`
	Multiple bool           `yaml:"multiple"`
	Options  []PromptOption `yaml:"options,omitempty"`
	Default  string         `yaml:"default,omitempty"`
	Help     string         `yaml:"help"`
	Optional bool           `yaml:"optional,omitempty"`
	Generate bool           `yaml:"generate,omitempty"`
}

type PromptOption struct {
	Blank    bool
	Checked  bool
	Name     string `yaml:"name"`
	Value    string `yaml:"value,omitempty"`
	Requires string `yaml:"requires,omitempty"`
}

type ParsedYaml map[string][]UserPrompt

// It will render as:
//
//	# The path to the VertexAI application credentials JSON file.
//	VERTEXAI_APPLICATION_CREDENTIALS=whatever-user-entered
func (up UserPrompt) String() string {
	helpLines := up.HelpLines()

	if len(helpLines) == 0 {
		return up.StringWithoutHelp()
	}

	return strings.Join(helpLines, "") + up.StringWithoutHelp()
}

func (up UserPrompt) HelpLines() []string {
	if up.Help == "" {
		return nil
	}

	// Account for multiline strings - also normalize if there's carriage returns
	helpNormalized := strings.ReplaceAll(up.Help, "\r\n", "\n")
	helpLines := strings.Split(helpNormalized, "\n")

	helpLinesResult := make([]string, len(helpLines)+1)
	helpLinesResult[0] = "#\n"

	for i, helpLine := range helpLines {
		helpLinesResult[i+1] = fmt.Sprintf("# %v\n", helpLine)
	}

	return helpLinesResult
}

func (up UserPrompt) StringWithoutHelp() string {
	var result strings.Builder

	quotedValue := strconv.Quote(up.Value)

	if up.Env != "" {
		if up.Commented || !up.Active {
			result.WriteString("# ")
		}

		result.WriteString(fmt.Sprintf("%s=%v", up.Env, quotedValue))
	} else {
		result.WriteString(fmt.Sprintf("# %s=%v", up.Key, quotedValue))
	}

	return result.String()
}

func (up UserPrompt) VarName() string {
	if up.Env != "" {
		return up.Env
	}

	return up.Key
}

func (up UserPrompt) SkipSaving() bool {
	return !up.Active && up.Value == up.Default
}

// HasEnvValue returns true if prompt has effective value when written to .env file
func (up UserPrompt) HasEnvValue() bool {
	return !up.Commented && up.Env != "" && up.Active
}

func (up UserPrompt) Valid() bool {
	return up.Optional || up.Value != ""
}

func (up UserPrompt) ShouldAsk() bool {
	return up.Active && !up.Hidden
}

func GatherUserPrompts(rootDir string, variables Variables) ([]UserPrompt, error) {
	yamlFiles, err := Discover(rootDir, 5)
	if err != nil {
		return nil, fmt.Errorf("Failed to discover task yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return nil, nil
	}

	allPrompts := make([]UserPrompt, 0)
	allPrompts = append(allPrompts, corePrompts...)

	for _, yamlFile := range yamlFiles {
		prompts, err := filePrompts(yamlFile)
		if err != nil {
			log.Debug(err)
			continue
		}

		allPrompts = append(allPrompts, prompts...)
	}

	allPrompts = promptsWithValues(allPrompts, variables)
	allPrompts = DetermineRequiredSections(allPrompts)

	return allPrompts, nil
}

func filePrompts(yamlFile string) ([]UserPrompt, error) {
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("Failed to read task yaml file %s: %w", yamlFile, err)
	}

	var fileParsed ParsedYaml

	if err = yaml.Unmarshal(data, &fileParsed); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal task yaml file %s: %w", yamlFile, err)
	}

	roots := rootSections(fileParsed)
	prompts := promptsSorted(fileParsed, roots)

	for p := range prompts {
		if slices.Contains(roots, prompts[p].Section) {
			prompts[p].Root = true
			prompts[p].Active = true
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

	return prompts, nil
}

func promptsSorted(fileParsed ParsedYaml, sections []string) []UserPrompt {
	sortedPrompts := make([]UserPrompt, 0)

	for _, section := range sections {
		for _, prompt := range fileParsed[section] {
			prompt.Section = section

			sortedPrompts = append(sortedPrompts, prompt)

			requiredPrompts := promptsSorted(fileParsed, childSections(prompt))
			sortedPrompts = append(sortedPrompts, requiredPrompts...)
		}
	}

	return sortedPrompts
}

// rootSections is used only for determining sort order of prompts.
// Use DetermineRequiredSections to determine whether given section is required.
func rootSections(fileParsed ParsedYaml) []string {
	keys := make(map[string]struct{})

	for key := range maps.Keys(fileParsed) {
		keys[key] = struct{}{}
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

// childSections is used only for determining sort order of prompts.
// Use DetermineRequiredSections to determine whether given section is required.
func childSections(prompt UserPrompt) []string {
	keys := make([]string, 0, len(prompt.Options))

	for _, option := range prompt.Options {
		if option.Requires != "" {
			keys = append(keys, option.Requires)
		}
	}

	return keys
}
