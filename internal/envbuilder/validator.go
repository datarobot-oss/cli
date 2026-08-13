// Copyright 2026 DataRobot, Inc. and its affiliates.
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
	"os"
	"slices"
	"strings"

	"github.com/datarobot/cli/internal/config/viperx"
)

// ValidationResult represents the validation status of a single variable.
type ValidationResult struct {
	Field   string // The environment variable name or key
	Value   string // The actual value (empty if not set)
	Valid   bool   // Whether the variable is valid
	Message string // Error message if invalid, or success message if valid
	Help    string // Optional help text describing the variable
}

// EnvironmentValidationError contains the results of validating environment configuration.
type EnvironmentValidationError struct {
	Results []ValidationResult
}

// HasErrors returns true if there are any validation errors.
func (r EnvironmentValidationError) HasErrors() bool {
	for _, result := range r.Results {
		if !result.Valid {
			return true
		}
	}

	return false
}

// Error implements the error interface for EnvironmentValidationError.
func (r EnvironmentValidationError) Error() string {
	if !r.HasErrors() {
		return ""
	}

	var builder strings.Builder

	builder.WriteString("Validation errors:\n")

	for _, result := range r.Results {
		if !result.Valid {
			builder.WriteString("  - ")
			builder.WriteString(result.Field)
			builder.WriteString(": ")
			builder.WriteString(result.Message)

			if result.Help != "" {
				builder.WriteString(" (")
				builder.WriteString(result.Help)
				builder.WriteString(")")
			}

			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// ValidateEnvironment validates that all required environment variables are set according to
// UserPrompts and core DataRobot requirements. It checks both the provided envValues map
// (which should contain .env file values) and environment variables (which override .env values).
//
// The validation process:
// 1. Determines which sections are active based on requires dependencies
// 2. Validates all required UserPrompts in active sections
// 3. Validates core DataRobot variables (DATAROBOT_ENDPOINT, DATAROBOT_API_TOKEN).
func ValidateEnvironment(repoRoot string, variables Variables) EnvironmentValidationError {
	return validateEnvironment(repoRoot, variables, true, true)
}

// ValidateEnvironmentFileOnly validates that all required environment variables are present
// as effective, uncommented entries in the .env file. It ignores OS environment variables,
// YAML defaults, and auto-generated secrets, so a variable that is satisfied only by a
// default or a generated value does not count as present. This is used for --if-needed
// skip checks where we want to know whether the .env file itself is complete.
//
// The PULUMI_CONFIG_PASSPHRASE viper-config value is an intentional non-file exception
// (see resolvePulumiPassphrase); it counts as present even though it is stored in the CLI
// config rather than the .env file.
func ValidateEnvironmentFileOnly(repoRoot string, variables Variables) EnvironmentValidationError {
	return validateEnvironment(repoRoot, variables, false, false)
}

func validateEnvironment(repoRoot string, variables Variables, includeEnv, synthesize bool) EnvironmentValidationError {
	result := EnvironmentValidationError{
		Results: make([]ValidationResult, 0),
	}

	// Gather all user prompts from the repository
	userPrompts, err := gatherUserPrompts(repoRoot, variables, includeEnv, synthesize)
	if err != nil {
		result.Results = append(result.Results, ValidationResult{
			Field:   "prompts",
			Valid:   false,
			Message: "Failed to gather user prompts: " + err.Error(),
		})

		return result
	}

	// Validate required prompts
	validatePrompts(&result, userPrompts)

	return result
}

// promptsWithValues updates slice of prompts with values from .env file contents
// and environment variables (environment variables take precedence).
// When includeEnv is false, OS environment variables are ignored and only
// .env file values (plus defaults and viper config) are used.
// When synthesize is false (presence-only mode), commented .env entries count as
// absent and YAML defaults are not applied.
func promptsWithValues(prompts []UserPrompt, variables Variables, includeEnv, synthesize bool) []UserPrompt {
	// Special handling for PULUMI_CONFIG_PASSPHRASE from viper config
	// This happens regardless of whether variables exist
	for p := range prompts {
		if prompts[p].Env == "PULUMI_CONFIG_PASSPHRASE" {
			prompts[p].Value = resolvePulumiPassphrase(prompts[p].Value, includeEnv)
		}
	}

	if len(variables) == 0 {
		return prompts
	}

	for p, prompt := range prompts {
		prompts[p] = resolvePromptValue(prompt, variables, includeEnv, synthesize)
	}

	return prompts
}

// resolvePulumiPassphrase determines the PULUMI_CONFIG_PASSPHRASE value.
// In file-only mode, always check viper config. Otherwise, prefer OS env over viper.
func resolvePulumiPassphrase(currentValue string, includeEnv bool) string {
	if !includeEnv {
		// File-only mode: ignore OS env, always check viper config
		if configValue := viperx.GetString("pulumi_config_passphrase"); configValue != "" {
			return configValue
		}

		return currentValue
	}

	// Normal mode: check OS env first, then viper config
	if _, ok := os.LookupEnv("PULUMI_CONFIG_PASSPHRASE"); ok {
		return currentValue
	}

	if configValue := viperx.GetString("pulumi_config_passphrase"); configValue != "" {
		return configValue
	}

	return currentValue
}

// resolvePromptValue determines the effective value for a prompt based on
// .env file contents and optionally OS environment variables.
// In presence-only mode (synthesize false), commented .env entries are treated as
// absent and YAML defaults are not applied, so only an uncommented file entry (or the
// PULUMI_CONFIG_PASSPHRASE viper-config exception) can satisfy the prompt.
func resolvePromptValue(prompt UserPrompt, variables Variables, includeEnv, synthesize bool) UserPrompt {
	if includeEnv {
		// Capture existing env var values (highest priority)
		if existingEnvValue, ok := os.LookupEnv(prompt.Env); ok {
			prompt.Value = existingEnvValue

			return prompt
		}
	}

	if v, found := variables.find(prompt); found {
		// In presence-only mode, a commented .env entry counts as absent: leave Value
		// unset so the prompt fails validation. Any prior value (e.g. the PULUMI
		// passphrase from viper config) is preserved.
		if !synthesize && v.Commented {
			return prompt
		}

		// .env file value overrides viper config
		prompt.Value = v.Value
		prompt.Commented = v.Commented

		return prompt
	}

	// Only apply YAML defaults when synthesizing; in presence-only mode an absent
	// variable stays absent.
	if synthesize && prompt.Value == "" {
		prompt.Value = prompt.Default
	}

	return prompt
}

func indexByName(value string) func(v Variable) bool {
	return func(v Variable) bool {
		return v.Name == value
	}
}

func (vv Variables) find(prompt UserPrompt) (Variable, bool) {
	if envIndex := slices.IndexFunc(vv, indexByName(prompt.Env)); envIndex != -1 {
		return vv[envIndex], true
	}

	if keyIndex := slices.IndexFunc(vv, indexByName(prompt.Key)); keyIndex != -1 {
		return vv[keyIndex], true
	}

	return Variable{}, false
}

// DetermineRequiredSections calculates which sections are required based on the
// requires dependencies in selected options.
func DetermineRequiredSections(userPrompts []UserPrompt) []UserPrompt {
	for p := range userPrompts {
		userPrompts[p].Active = userPrompts[p].Root
	}

	activeSections := make(map[string]struct{})

	// Process prompts in order to determine which sections are enabled
	for _, prompt := range userPrompts {
		for _, section := range getRequiredSections(prompt) {
			activeSections[section] = struct{}{}
		}
	}

	for p, prompt := range userPrompts {
		if _, active := activeSections[prompt.Section]; active {
			userPrompts[p].Active = true
		}
	}

	duplicates := make(map[string]struct{})

	for p, prompt := range userPrompts {
		if !userPrompts[p].Active {
			continue
		}

		varName := prompt.VarName()

		if varName == "" {
			continue
		}

		if _, ok := duplicates[varName]; ok {
			userPrompts[p].Active = false
		}

		duplicates[varName] = struct{}{}
	}

	return userPrompts
}

// getRequiredSections checks if any options with requires are selected and returns those sections.
func getRequiredSections(prompt UserPrompt) []string {
	if len(prompt.Options) == 0 {
		return nil
	}

	requiredSections := make([]string, 0)
	selectedValues := strings.Split(prompt.Value, ",")

	for _, option := range prompt.Options {
		if option.Requires == "" {
			continue
		}

		if isOptionSelected(option, selectedValues) {
			requiredSections = append(requiredSections, option.Requires)
		}
	}

	return requiredSections
}

// isOptionSelected checks if an option is selected based on the selected values.
func isOptionSelected(option PromptOption, selectedValues []string) bool {
	if option.Value != "" {
		return slices.Contains(selectedValues, option.Value)
	}

	return slices.Contains(selectedValues, option.Name)
}

// validatePrompts validates all required prompts in active sections.
func validatePrompts(result *EnvironmentValidationError, userPrompts []UserPrompt) {
	for _, prompt := range userPrompts {
		if !prompt.Active || prompt.Optional {
			continue
		}

		if !prompt.Valid() {
			result.Results = append(result.Results, ValidationResult{
				Field:   prompt.Env,
				Value:   "",
				Valid:   false,
				Message: "Required variable is not set.",
				Help:    prompt.Help,
			})
		} else {
			result.Results = append(result.Results, ValidationResult{
				Field:   prompt.Env,
				Value:   prompt.Value,
				Valid:   true,
				Message: "Variable is set.",
				Help:    prompt.Help,
			})
		}
	}
}
