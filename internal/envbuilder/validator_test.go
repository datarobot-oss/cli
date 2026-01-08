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
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestValidationResult(t *testing.T) {
	t.Run("Valid result", func(t *testing.T) {
		result := ValidationResult{
			Value: "test-value",
			Valid: true,
		}

		if !result.Valid {
			t.Error("Expected Valid to be true")
		}

		if result.Value != "test-value" {
			t.Errorf("Expected Value to be 'test-value', got '%s'", result.Value)
		}
	})

	t.Run("Invalid result with help", func(t *testing.T) {
		result := ValidationResult{
			Valid: false,
			Help:  "This variable is required for authentication",
		}

		if result.Valid {
			t.Error("Expected Valid to be false")
		}

		if result.Help != "This variable is required for authentication" {
			t.Errorf("Expected Help text, got '%s'", result.Help)
		}
	})
}

func TestEnvironmentValidationError_HasErrors(t *testing.T) {
	t.Run("No errors", func(t *testing.T) {
		err := EnvironmentValidationError{
			Results: []ValidationResult{
				{Field: "VAR1", Valid: true},
				{Field: "VAR2", Valid: true},
			},
		}

		if err.HasErrors() {
			t.Error("Expected HasErrors to be false when all results are valid")
		}
	})

	t.Run("Has errors", func(t *testing.T) {
		err := EnvironmentValidationError{
			Results: []ValidationResult{
				{Field: "VAR1", Valid: true},
				{Field: "VAR2", Valid: false, Message: "not set"},
			},
		}

		if !err.HasErrors() {
			t.Error("Expected HasErrors to be true when some results are invalid")
		}
	})

	t.Run("Empty results", func(t *testing.T) {
		err := EnvironmentValidationError{
			Results: []ValidationResult{},
		}

		if err.HasErrors() {
			t.Error("Expected HasErrors to be false for empty results")
		}
	})
}

func TestEnvironmentValidationError_Error(t *testing.T) {
	t.Run("No errors returns empty string", func(t *testing.T) {
		err := EnvironmentValidationError{
			Results: []ValidationResult{
				{Field: "VAR1", Valid: true},
			},
		}

		if err.Error() != "" {
			t.Errorf("Expected empty error string, got '%s'", err.Error())
		}
	})

	t.Run("Formats errors correctly", func(t *testing.T) {
		err := EnvironmentValidationError{
			Results: []ValidationResult{
				{Field: "VAR1", Valid: true},
				{Field: "VAR2", Valid: false, Message: "not set", Help: "Help text"},
				{Field: "VAR3", Valid: false, Message: "invalid format"},
			},
		}

		errMsg := err.Error()

		if errMsg == "" {
			t.Error("Expected non-empty error message")
		}

		// Check that error message contains the invalid fields
		if !contains(errMsg, "VAR2") {
			t.Error("Expected error message to contain VAR2")
		}

		if !contains(errMsg, "VAR3") {
			t.Error("Expected error message to contain VAR3")
		}

		// Should not contain valid field
		if contains(errMsg, "VAR1") {
			t.Error("Expected error message to not contain VAR1")
		}

		// Check help text is included
		if !contains(errMsg, "Help text") {
			t.Error("Expected error message to contain help text")
		}
	})
}

func TestPromptsWithValues(t *testing.T) {
	t.Run("Merges .env values", func(t *testing.T) {
		variables := []Variable{
			{Name: "VAR1", Value: "value1"},
			{Name: "VAR2", Value: "value2"},
		}

		prompts := []UserPrompt{
			{Env: "VAR1"},
			{Env: "VAR2"},
		}

		result := promptsWithValues(prompts, variables)

		if result[0].Value != "value1" {
			t.Errorf("Expected VAR1 to be 'value1', got '%s'", result[0].Value)
		}

		if result[1].Value != "value2" {
			t.Errorf("Expected VAR2 to be 'value2', got '%s'", result[1].Value)
		}
	})

	t.Run("Environment variables override .env values", func(t *testing.T) {
		// Set environment variable
		os.Setenv("TEST_OVERRIDE", "from-env")

		defer os.Unsetenv("TEST_OVERRIDE")

		variables := []Variable{
			{Name: "TEST_OVERRIDE", Value: "from-dotenv"},
		}

		prompts := []UserPrompt{
			{Env: "TEST_OVERRIDE"},
		}

		result := promptsWithValues(prompts, variables)

		if result[0].Value != "from-env" {
			t.Errorf("Expected TEST_OVERRIDE to be overridden to 'from-env', got '%s'", result[0].Value)
		}
	})
}

func TestIsOptionSelected(t *testing.T) {
	t.Run("Matches by Value", func(t *testing.T) {
		option := PromptOption{
			Name:  "Option Name",
			Value: "opt-value",
		}

		selectedValues := []string{"opt-value", "other"}

		if !isOptionSelected(option, selectedValues) {
			t.Error("Expected option to be selected by value")
		}
	})

	t.Run("Matches by Name when Value is empty", func(t *testing.T) {
		option := PromptOption{
			Name: "Option Name",
		}

		selectedValues := []string{"Option Name", "other"}

		if !isOptionSelected(option, selectedValues) {
			t.Error("Expected option to be selected by name")
		}
	})

	t.Run("Not selected", func(t *testing.T) {
		option := PromptOption{
			Name:  "Option Name",
			Value: "opt-value",
		}

		selectedValues := []string{"other-value"}

		if isOptionSelected(option, selectedValues) {
			t.Error("Expected option to not be selected")
		}
	})
}

func TestGetRequiredSections(t *testing.T) {
	t.Run("No options does nothing", func(t *testing.T) {
		prompt := UserPrompt{
			Value:   "value",
			Options: []PromptOption{},
		}

		sections := getRequiredSections(prompt)

		if len(sections) != 0 {
			t.Error("Expected no required sections")
		}
	})

	t.Run("Returns section when option is selected", func(t *testing.T) {
		prompt := UserPrompt{
			Value: "yes",
			Options: []PromptOption{
				{
					Name:     "Enable Feature",
					Value:    "yes",
					Requires: "feature-section",
				},
			},
		}

		sections := getRequiredSections(prompt)

		if !slices.Contains(sections, "feature-section") {
			t.Error("Expected feature-section to be enabled")
		}
	})

	t.Run("Multiple selections enable multiple sections", func(t *testing.T) {
		prompt := UserPrompt{
			Value: "opt1,opt2,opt3",
			Options: []PromptOption{
				{Value: "opt1", Requires: "section1"},
				{Value: "opt2", Requires: "section2"},
				{Value: "opt3"}, // No requires
			},
		}

		sections := getRequiredSections(prompt)

		if !slices.Contains(sections, "section1") {
			t.Error("Expected section1 to be enabled")
		}

		if !slices.Contains(sections, "section2") {
			t.Error("Expected section2 to be enabled")
		}
	})
}

func TestValidatePrompts(t *testing.T) {
	t.Run("Validates required prompts in active sections", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section:  "root",
				Env:      "REQUIRED_VAR",
				Optional: false,
				Active:   true,
			},
			{
				Section:  "inactive",
				Env:      "INACTIVE_VAR",
				Optional: false,
			},
		}

		prompts = promptsWithValues(prompts, []Variable{
			{Name: "REQUIRED_VAR", Value: "value"},
		})

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts)

		if len(result.Results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(result.Results))
		}

		if !result.Results[0].Valid {
			t.Error("Expected REQUIRED_VAR to be valid")
		}
	})

	t.Run("Skips optional prompts", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section:  "root",
				Env:      "OPTIONAL_VAR",
				Optional: true,
			},
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts)

		if len(result.Results) != 0 {
			t.Errorf("Expected 0 results for optional prompt, got %d", len(result.Results))
		}
	})

	t.Run("Marks missing required variables as invalid", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section:  "root",
				Env:      "MISSING_VAR",
				Optional: false,
				Help:     "This variable is required",
				Active:   true,
			},
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts)

		if len(result.Results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(result.Results))
		}

		if result.Results[0].Valid {
			t.Error("Expected MISSING_VAR to be invalid")
		}

		if result.Results[0].Help != "This variable is required" {
			t.Errorf("Expected help text, got '%s'", result.Results[0].Help)
		}
	})
}

func TestValidateEnvironment_Integration(t *testing.T) {
	t.Run("Validates core variables even with empty prompts", func(t *testing.T) {
		result := ValidateEnvironment("/nonexistent/path", Variables{})

		verifyEmptyPromptsValidation(t, result)
	})

	t.Run("Full validation with test data", func(t *testing.T) {
		tmpDir, variables := setupTestEnvironment(t)

		result := ValidateEnvironment(tmpDir, variables)

		verifyFullValidation(t, result)
	})
}

func verifyEmptyPromptsValidation(t *testing.T, result EnvironmentValidationError) {
	t.Helper()

	if !result.HasErrors() {
		t.Error("Expected validation to have errors for missing core variables")
	}

	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results for core variables, got %d", len(result.Results))
	}

	for _, r := range result.Results {
		if r.Valid {
			t.Errorf("Expected %s to be invalid (not set)", r.Field)
		}

		if r.Field != "DATAROBOT_ENDPOINT" && r.Field != "DATAROBOT_API_TOKEN" {
			t.Errorf("Unexpected field in results: %s", r.Field)
		}
	}
}

func setupTestEnvironment(t *testing.T) (string, Variables) {
	t.Helper()

	tmpDir := t.TempDir()

	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	promptsFile := filepath.Join(promptsDir, "prompts.yaml")
	promptsContent := `- section: test
  prompts:
    - key: test-var
      env: TEST_VAR
      help: Test variable
      optional: false
`

	if err := os.WriteFile(promptsFile, []byte(promptsContent), 0o644); err != nil {
		t.Fatalf("Failed to write prompts file: %v", err)
	}

	variables := []Variable{
		{Name: "TEST_VAR", Value: "test-value"},
		{Name: "DATAROBOT_ENDPOINT", Value: "https://app.datarobot.com"},
		{Name: "DATAROBOT_API_TOKEN", Value: "token123"},
	}

	// Set as actual environment variables to ensure they're found regardless of viper state
	t.Setenv("TEST_VAR", "test-value")
	t.Setenv("DATAROBOT_ENDPOINT", "https://app.datarobot.com")
	t.Setenv("DATAROBOT_API_TOKEN", "token123")

	return tmpDir, variables
}

func verifyFullValidation(t *testing.T, result EnvironmentValidationError) {
	t.Helper()

	if result.HasErrors() {
		t.Errorf("Expected no errors, got: %s", result.Error())
	}

	if len(result.Results) < 2 {
		t.Errorf("Expected at least 2 core variable results, got %d", len(result.Results))
	}

	for _, r := range result.Results {
		if !r.Valid {
			t.Errorf("Expected all results to be valid, but %s is invalid: %s", r.Field, r.Message)
		}
	}
}

func TestRootSections(t *testing.T) {
	t.Run("Initializes with root sections", func(t *testing.T) {
		fileParsed := ParsedYaml{
			"root1": {},
			"root2": {},
		}

		result := rootSections(fileParsed)

		if !slices.Contains(result, "root1") {
			t.Error("Expected root1 to be required")
		}

		if !slices.Contains(result, "root2") {
			t.Error("Expected root2 to be required")
		}
	})
}

func TestDetermineRequiredSections(t *testing.T) {
	t.Run("Enables dependent sections based on option selection", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Env:     "FEATURE_TOGGLE",
				Root:    true,
				Value:   "yes",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "yes",
						Requires: "feature-config",
					},
				},
			},
			{Section: "feature-config"},
		}

		prompts = DetermineRequiredSections(prompts)

		if !prompts[0].Active {
			t.Error("Expected root to be required")
		}

		if !prompts[1].Active {
			t.Error("Expected feature-config to be enabled")
		}
	})

	t.Run("Does not enable sections for unselected options", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Env:     "FEATURE_TOGGLE",
				Root:    true,
				Value:   "yes",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "yes",
						Requires: "feature-config",
					},
					{
						Name:     "Disable",
						Value:    "no",
						Requires: "disable-config",
					},
				},
			},
			{Section: "feature-config"},
			{Section: "disable-config"},
		}

		prompts = DetermineRequiredSections(prompts)

		if !prompts[1].Active {
			t.Error("Expected feature-config to be enabled")
		}

		if prompts[2].Active {
			t.Error("Expected disable-config to not be enabled")
		}
	})
}

func TestDetermineRequiredSectionsDuplicates(t *testing.T) {
	t.Run("Deactivates prompts with same Env, active first", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Root:    true,
				Env:     "DUP",
				Value:   "root dup",
			},
			{
				Section: "root",
				Root:    true,
				Env:     "FEATURE_TOGGLE",
				Value:   "one,two",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "one",
						Requires: "one-config",
					},
					{
						Name:     "Disable",
						Value:    "two",
						Requires: "two-config",
					},
				},
			},
			{Section: "one-config", Env: "DUP", Value: "one dup"},
			{Section: "two-config", Env: "DUP", Value: "two dup"},
		}

		prompts = DetermineRequiredSections(prompts)

		if prompts[0].String() != `DUP="root dup"` {
			t.Errorf("Expected root dup [0] to be enabled, got: %s", prompts[0])
		}

		if prompts[2].String() != `# DUP="one dup"` {
			t.Errorf("Expected other dup [2] to not be enabled, got: %s", prompts[2])
		}

		if prompts[3].String() != `# DUP="two dup"` {
			t.Errorf("Expected other dup [3] to not be enabled, got: %s", prompts[3])
		}
	})

	t.Run("Deactivates prompts with same Env, active middle", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Root:    true,
				Env:     "FEATURE_TOGGLE",
				Value:   "two",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "one",
						Requires: "one-config",
					},
					{
						Name:     "Disable",
						Value:    "two",
						Requires: "two-config",
					},
				},
			},
			{Section: "one-config", Env: "DUP", Value: "one dup"},
			{Section: "two-config", Env: "DUP", Value: "two dup"},
			{
				Section: "root", Root: true,
				Env:   "DUP",
				Value: "root dup",
			},
		}

		prompts = DetermineRequiredSections(prompts)

		if prompts[1].String() != `# DUP="one dup"` {
			t.Errorf("Expected other dup [1] to not be enabled, got: %s", prompts[1])
		}

		if prompts[2].String() != `DUP="two dup"` {
			t.Errorf("Expected other dup [2] to not be enabled, got: %s", prompts[2])
		}

		if prompts[3].String() != `# DUP="root dup"` {
			t.Errorf("Expected root dup [3] to be enabled, got: %s", prompts[3])
		}
	})

	t.Run("Deactivates prompts with same Env, active last", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Root:    true,
				Env:     "FEATURE_TOGGLE",
				Value:   "",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "one",
						Requires: "one-config",
					},
					{
						Name:     "Disable",
						Value:    "two",
						Requires: "two-config",
					},
				},
			},
			{Section: "one-config", Env: "DUP", Value: "one dup"},
			{Section: "two-config", Env: "DUP", Value: "two dup"},
			{
				Section: "root", Root: true,
				Env: "DUP", Value: "root dup",
			},
		}

		prompts = DetermineRequiredSections(prompts)

		if prompts[1].String() != `# DUP="one dup"` {
			t.Errorf("Expected other dup [1] to not be enabled, got: %s", prompts[1])
		}

		if prompts[2].String() != `# DUP="two dup"` {
			t.Errorf("Expected other dup [2] to not be enabled, got: %s", prompts[2])
		}

		if prompts[3].String() != `DUP="root dup"` {
			t.Errorf("Expected root dup [3] to be enabled, got: %s", prompts[3])
		}
	})
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
