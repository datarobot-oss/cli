// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"os"
	"slices"
	"strings"
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

	builder.WriteString("validation errors:\n")

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
// 3. Validates core DataRobot variables (DATAROBOT_ENDPOINT, DATAROBOT_API_TOKEN)
func ValidateEnvironment(repoRoot string, variables Variables) EnvironmentValidationError {
	result := EnvironmentValidationError{
		Results: make([]ValidationResult, 0),
	}

	// Gather all user prompts from the repository
	userPrompts, err := GatherUserPrompts(repoRoot, variables)
	if err != nil {
		result.Results = append(result.Results, ValidationResult{
			Field:   "prompts",
			Valid:   false,
			Message: "failed to gather user prompts: " + err.Error(),
		})

		return result
	}

	// Validate required prompts
	validatePrompts(&result, userPrompts)

	// Validate core DataRobot variables
	validateCoreVariables(&result, variables.valuesMap())

	return result
}

// promptsWithValues updates slice of prompts with values from .env file contents
// and environment variables (environment variables take precedence).
func promptsWithValues(prompts []UserPrompt, variables Variables) []UserPrompt {
	if len(variables) == 0 {
		return prompts
	}

	for p, prompt := range prompts {
		// Capture existing env var values
		if existingEnvValue, ok := os.LookupEnv(prompt.Env); ok {
			prompt.Value = existingEnvValue
		} else if v, found := variables.find(prompt); found {
			prompt.Value = v.Value
			prompt.Commented = v.Commented
		} else {
			prompt.Value = prompt.Default
		}

		prompts[p] = prompt
	}

	return prompts
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

func (vv Variables) valuesMap() map[string]string {
	envValues := make(map[string]string)

	for _, v := range vv {
		if v.Name != "" && !v.Commented {
			envValues[v.Name] = v.Value
		}
	}

	return envValues
}

// determineRequiredSections calculates which sections are required based on the
// requires dependencies in selected options.
func determineRequiredSections(userPrompts []UserPrompt) []UserPrompt {
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
				Message: "required variable is not set",
				Help:    prompt.Help,
			})
		} else {
			result.Results = append(result.Results, ValidationResult{
				Field:   prompt.Env,
				Value:   prompt.Value,
				Valid:   true,
				Message: "variable is set",
				Help:    prompt.Help,
			})
		}
	}
}

// validateCoreVariables validates the core DataRobot variables that must always be present.
func validateCoreVariables(result *EnvironmentValidationError, effectiveValues map[string]string) {
	requiredVars := []string{"DATAROBOT_ENDPOINT", "DATAROBOT_API_TOKEN"}

	for _, requiredVar := range requiredVars {
		value := effectiveValues[requiredVar]

		// Check environment variable (overrides .env file)
		if envValue, ok := os.LookupEnv(requiredVar); ok {
			value = envValue
		}

		if value == "" {
			result.Results = append(result.Results, ValidationResult{
				Field:   requiredVar,
				Value:   "",
				Valid:   false,
				Message: "required DataRobot variable is not set",
				Help:    "Set this variable in your .env file or run `dr dotenv setup` to configure it.",
			})
		} else {
			result.Results = append(result.Results, ValidationResult{
				Field:   requiredVar,
				Value:   value,
				Valid:   true,
				Message: "DataRobot variable is set",
			})
		}
	}
}
