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

package envbuilder

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/log"
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

// UserPrompt represents a configuration prompt that can be displayed to users
// during the dotenv setup wizard. Prompts are defined in YAML files within the .datarobot
// directory of a given template.
type UserPrompt struct {
	Section string
	Root    bool
	// Active indicates if this prompt should be processed (based on conditional logic).
	Active bool
	// Commented indicates if the variable should be commented out in the .env file. This
	// can be used for variables that the user may want to set but are not required.
	Commented bool
	// Value is the current value for this prompt (from .env, environment, or user input).
	Value string
	// Hidden indicates if this prompt should never be shown to users (e.g., core variables).
	Hidden bool

	// Env is the environment variable name to set (e.g., "DATABASE_URL").
	Env string `yaml:"env"`
	// Key is an alternative identifier when Env is not set (written as comment).
	Key string `yaml:"key"`
	// Type is the prompt type: "string" (default) or "secret_string" (masked input).
	Type PromptType `yaml:"type"`
	// Multiple allows selecting multiple options (checkbox-style) when Options is set.
	Multiple bool `yaml:"multiple"`
	// Options provides a list of choices for selection-style prompts.
	Options []PromptOption `yaml:"options,omitempty"`
	// Default is the initial value for this prompt. Prompts with defaults are
	// skipped during the wizard unless the value differs or AlwaysPrompt is set.
	Default string `yaml:"default,omitempty"`
	// Help is the description text shown to users when prompting for input.
	Help string `yaml:"help"`
	// Optional allows the prompt to be skipped without providing a value.
	Optional bool `yaml:"optional,omitempty"`
	// Generate auto-generates a cryptographic random value for secret_string types.
	Generate bool `yaml:"generate,omitempty"`
	// AlwaysPrompt forces the prompt to be shown even when a default value is set.
	// Use this for prompts where users should consciously confirm or change the default.
	// This does not affect prompts with options that have "requires" fields -
	// those prompts are always shown regardless of defaults. This also does not
	// affect prompts that are required due to conditional logic, or hidden prompts.
	AlwaysPrompt bool `yaml:"always_prompt,omitempty"`
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

// HasRequiresOptions returns true if any option has a requires field set.
func (up UserPrompt) HasRequiresOptions() bool {
	for _, opt := range up.Options {
		if opt.Requires != "" {
			return true
		}
	}

	return false
}

// ShouldAsk returns true if this prompt should be shown to the user.
// Prompts with defaults are skipped unless AlwaysPrompt is set, showAll is true,
// or the prompt has options with requires (which control conditional sections).
func (up UserPrompt) ShouldAsk(showAll bool) bool {
	if !up.Active || up.Hidden {
		return false
	}

	// If showAll flag is set, show all active non-hidden prompts
	if showAll {
		return true
	}

	// If prompt has always_prompt: true, always show it
	if up.AlwaysPrompt {
		return true
	}

	// Always show prompts with requires options - they control conditional sections
	if up.HasRequiresOptions() {
		return true
	}

	// Skip prompts that have a default and value equals default (not user-modified)
	if up.Default != "" && up.Value == up.Default {
		return false
	}

	return true
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
