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
func ValidateEnvironment(repoRoot string, envValues map[string]string) EnvironmentValidationError {
	result := EnvironmentValidationError{
		Results: make([]ValidationResult, 0),
	}

	// Gather all user prompts from the repository
	userPrompts, rootSections, err := GatherUserPrompts(repoRoot)
	if err != nil {
		result.Results = append(result.Results, ValidationResult{
			Field:   "prompts",
			Valid:   false,
			Message: "failed to gather user prompts: " + err.Error(),
		})

		return result
	}

	// Create effective values by merging .env and environment variables
	effectiveValues := buildEffectiveValues(envValues, userPrompts)

	// Determine which sections are required
	requiredSections := determineRequiredSections(userPrompts, rootSections, effectiveValues)

	// Validate required prompts
	validatePrompts(&result, userPrompts, requiredSections, effectiveValues)

	// Validate core DataRobot variables
	validateCoreVariables(&result, effectiveValues)

	return result
}

// buildEffectiveValues creates a map of environment values by merging .env file values
// with environment variables (environment variables take precedence).
func buildEffectiveValues(envValues map[string]string, userPrompts []UserPrompt) map[string]string {
	effectiveValues := make(map[string]string)

	for k, v := range envValues {
		effectiveValues[k] = v
	}

	// Environment variables override .env file values
	for _, prompt := range userPrompts {
		if prompt.Env != "" {
			if existingValue, ok := os.LookupEnv(prompt.Env); ok {
				effectiveValues[prompt.Env] = existingValue
			}
		}
	}

	return effectiveValues
}

// determineRequiredSections calculates which sections are required based on the
// requires dependencies in selected options.
func determineRequiredSections(
	userPrompts []UserPrompt,
	rootSections []string,
	effectiveValues map[string]string,
) map[string]bool {
	requiredSections := make(map[string]bool)

	for _, root := range rootSections {
		requiredSections[root] = true
	}

	// Process prompts in order to determine which sections are enabled
	for _, prompt := range userPrompts {
		if !requiredSections[prompt.Section] {
			continue
		}

		envKey := getEnvKey(prompt)
		value, hasValue := effectiveValues[envKey]

		if hasValue {
			enableRequiredSections(prompt, value, requiredSections)
		}
	}

	return requiredSections
}

// getEnvKey returns the environment key for a prompt.
func getEnvKey(prompt UserPrompt) string {
	if prompt.Env != "" {
		return prompt.Env
	}

	return "# " + prompt.Key
}

// enableRequiredSections checks if any options with requires are selected and enables those sections.
func enableRequiredSections(prompt UserPrompt, value string, requiredSections map[string]bool) {
	if len(prompt.Options) == 0 {
		return
	}

	selectedValues := strings.Split(value, ",")

	for _, option := range prompt.Options {
		if option.Requires == "" {
			continue
		}

		if isOptionSelected(option, selectedValues) {
			requiredSections[option.Requires] = true
		}
	}
}

// isOptionSelected checks if an option is selected based on the selected values.
func isOptionSelected(option PromptOption, selectedValues []string) bool {
	if option.Value != "" {
		return slices.Contains(selectedValues, option.Value)
	}

	return slices.Contains(selectedValues, option.Name)
}

// validatePrompts validates all required prompts in active sections.
func validatePrompts(
	result *EnvironmentValidationError,
	userPrompts []UserPrompt,
	requiredSections map[string]bool,
	effectiveValues map[string]string,
) {
	for _, prompt := range userPrompts {
		if !requiredSections[prompt.Section] || prompt.Optional {
			continue
		}

		envKey := getEnvKey(prompt)
		value, hasValue := effectiveValues[envKey]

		if !hasValue || value == "" {
			result.Results = append(result.Results, ValidationResult{
				Field:   envKey,
				Value:   "",
				Valid:   false,
				Message: "required variable is not set",
				Help:    prompt.Help,
			})
		} else {
			result.Results = append(result.Results, ValidationResult{
				Field:   envKey,
				Value:   value,
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
