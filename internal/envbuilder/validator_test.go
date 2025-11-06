package envbuilder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidationResult(t *testing.T) {
	t.Run("Valid result", func(t *testing.T) {
		result := ValidationResult{
			Field:   "TEST_VAR",
			Value:   "test-value",
			Valid:   true,
			Message: "variable is set",
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
			Field:   "MISSING_VAR",
			Value:   "",
			Valid:   false,
			Message: "required variable is not set",
			Help:    "This variable is required for authentication",
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

func TestBuildEffectiveValues(t *testing.T) {
	t.Run("Merges .env values", func(t *testing.T) {
		envValues := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		prompts := []UserPrompt{
			{Env: "VAR1"},
			{Env: "VAR2"},
		}

		result := buildEffectiveValues(envValues, prompts)

		if result["VAR1"] != "value1" {
			t.Errorf("Expected VAR1 to be 'value1', got '%s'", result["VAR1"])
		}

		if result["VAR2"] != "value2" {
			t.Errorf("Expected VAR2 to be 'value2', got '%s'", result["VAR2"])
		}
	})

	t.Run("Environment variables override .env values", func(t *testing.T) {
		// Set environment variable
		os.Setenv("TEST_OVERRIDE", "from-env")
		defer os.Unsetenv("TEST_OVERRIDE")

		envValues := map[string]string{
			"TEST_OVERRIDE": "from-dotenv",
		}

		prompts := []UserPrompt{
			{Env: "TEST_OVERRIDE"},
		}

		result := buildEffectiveValues(envValues, prompts)

		if result["TEST_OVERRIDE"] != "from-env" {
			t.Errorf("Expected TEST_OVERRIDE to be overridden to 'from-env', got '%s'", result["TEST_OVERRIDE"])
		}
	})

	t.Run("Skips prompts without Env field", func(t *testing.T) {
		envValues := map[string]string{
			"VAR1": "value1",
		}

		prompts := []UserPrompt{
			{Env: "VAR1"},
			{Key: "some-key"}, // No Env field
		}

		result := buildEffectiveValues(envValues, prompts)

		if len(result) != 1 {
			t.Errorf("Expected 1 value, got %d", len(result))
		}
	})
}

func TestGetEnvKey(t *testing.T) {
	t.Run("Returns Env when present", func(t *testing.T) {
		prompt := UserPrompt{
			Env: "MY_VAR",
			Key: "my-key",
		}

		key := getEnvKey(prompt)

		if key != "MY_VAR" {
			t.Errorf("Expected 'MY_VAR', got '%s'", key)
		}
	})

	t.Run("Returns commented Key when Env is empty", func(t *testing.T) {
		prompt := UserPrompt{
			Key: "my-key",
		}

		key := getEnvKey(prompt)

		if key != "# my-key" {
			t.Errorf("Expected '# my-key', got '%s'", key)
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

func TestEnableRequiredSections(t *testing.T) {
	t.Run("No options does nothing", func(t *testing.T) {
		prompt := UserPrompt{
			Options: []PromptOption{},
		}

		requiredSections := map[string]bool{
			"root": true,
		}

		enableRequiredSections(prompt, "value", requiredSections)

		if len(requiredSections) != 1 {
			t.Error("Expected no changes to requiredSections")
		}
	})

	t.Run("Enables section when option is selected", func(t *testing.T) {
		prompt := UserPrompt{
			Options: []PromptOption{
				{
					Name:     "Enable Feature",
					Value:    "yes",
					Requires: "feature-section",
				},
			},
		}

		requiredSections := map[string]bool{
			"root": true,
		}

		enableRequiredSections(prompt, "yes", requiredSections)

		if !requiredSections["feature-section"] {
			t.Error("Expected feature-section to be enabled")
		}
	})

	t.Run("Multiple selections enable multiple sections", func(t *testing.T) {
		prompt := UserPrompt{
			Options: []PromptOption{
				{Value: "opt1", Requires: "section1"},
				{Value: "opt2", Requires: "section2"},
				{Value: "opt3"}, // No requires
			},
		}

		requiredSections := map[string]bool{
			"root": true,
		}

		enableRequiredSections(prompt, "opt1,opt2,opt3", requiredSections)

		if !requiredSections["section1"] {
			t.Error("Expected section1 to be enabled")
		}

		if !requiredSections["section2"] {
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
			},
			{
				Section:  "inactive",
				Env:      "INACTIVE_VAR",
				Optional: false,
			},
		}

		requiredSections := map[string]bool{
			"root": true,
		}

		effectiveValues := map[string]string{
			"REQUIRED_VAR": "value",
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts, requiredSections, effectiveValues)

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

		requiredSections := map[string]bool{
			"root": true,
		}

		effectiveValues := map[string]string{}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts, requiredSections, effectiveValues)

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
			},
		}

		requiredSections := map[string]bool{
			"root": true,
		}

		effectiveValues := map[string]string{}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validatePrompts(result, prompts, requiredSections, effectiveValues)

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

func TestValidateCoreVariables(t *testing.T) {
	t.Run("Validates DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN", func(t *testing.T) {
		effectiveValues := map[string]string{
			"DATAROBOT_ENDPOINT":  "https://app.datarobot.com",
			"DATAROBOT_API_TOKEN": "token123",
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validateCoreVariables(result, effectiveValues)

		if len(result.Results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(result.Results))
		}

		for _, r := range result.Results {
			if !r.Valid {
				t.Errorf("Expected %s to be valid", r.Field)
			}
		}
	})

	t.Run("Marks missing core variables as invalid", func(t *testing.T) {
		effectiveValues := map[string]string{
			"DATAROBOT_ENDPOINT": "https://app.datarobot.com",
			// DATAROBOT_API_TOKEN is missing
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validateCoreVariables(result, effectiveValues)

		if len(result.Results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(result.Results))
		}

		var tokenResult *ValidationResult

		for i := range result.Results {
			if result.Results[i].Field == "DATAROBOT_API_TOKEN" {
				tokenResult = &result.Results[i]
				break
			}
		}

		if tokenResult == nil {
			t.Fatal("Expected DATAROBOT_API_TOKEN result")
		}

		if tokenResult.Valid {
			t.Error("Expected DATAROBOT_API_TOKEN to be invalid")
		}
	})

	t.Run("Environment variables override .env values", func(t *testing.T) {
		os.Setenv("DATAROBOT_ENDPOINT", "https://env.datarobot.com")
		defer os.Unsetenv("DATAROBOT_ENDPOINT")

		effectiveValues := map[string]string{
			"DATAROBOT_ENDPOINT":  "https://dotenv.datarobot.com",
			"DATAROBOT_API_TOKEN": "token123",
		}

		result := &EnvironmentValidationError{
			Results: make([]ValidationResult, 0),
		}

		validateCoreVariables(result, effectiveValues)

		var endpointResult *ValidationResult

		for i := range result.Results {
			if result.Results[i].Field == "DATAROBOT_ENDPOINT" {
				endpointResult = &result.Results[i]
				break
			}
		}

		if endpointResult == nil {
			t.Fatal("Expected DATAROBOT_ENDPOINT result")
		}

		if endpointResult.Value != "https://env.datarobot.com" {
			t.Errorf("Expected value from environment, got '%s'", endpointResult.Value)
		}
	})
}

func TestValidateEnvironment_Integration(t *testing.T) {
	t.Run("Returns error when GatherUserPrompts fails", func(t *testing.T) {
		result := ValidateEnvironment("/nonexistent/path", map[string]string{})

		if !result.HasErrors() {
			t.Error("Expected validation to fail for nonexistent path")
		}

		if len(result.Results) == 0 {
			t.Error("Expected at least one error result")
		}

		if result.Results[0].Field != "prompts" {
			t.Error("Expected error about gathering prompts")
		}
	})

	t.Run("Full validation with test data", func(t *testing.T) {
		// Create a temporary directory with test prompts
		tmpDir := t.TempDir()

		promptsDir := filepath.Join(tmpDir, "prompts")
		if err := os.MkdirAll(promptsDir, 0755); err != nil {
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

		if err := os.WriteFile(promptsFile, []byte(promptsContent), 0644); err != nil {
			t.Fatalf("Failed to write prompts file: %v", err)
		}

		envValues := map[string]string{
			"TEST_VAR":              "test-value",
			"DATAROBOT_ENDPOINT":    "https://app.datarobot.com",
			"DATAROBOT_API_TOKEN":   "token123",
		}

		result := ValidateEnvironment(tmpDir, envValues)

		if result.HasErrors() {
			t.Errorf("Expected no errors, got: %s", result.Error())
		}

		// Should have results for TEST_VAR + 2 core variables
		if len(result.Results) < 3 {
			t.Errorf("Expected at least 3 results, got %d", len(result.Results))
		}
	})
}

func TestDetermineRequiredSections(t *testing.T) {
	t.Run("Initializes with root sections", func(t *testing.T) {
		prompts := []UserPrompt{}
		rootSections := []string{"root1", "root2"}
		effectiveValues := map[string]string{}

		result := determineRequiredSections(prompts, rootSections, effectiveValues)

		if !result["root1"] {
			t.Error("Expected root1 to be required")
		}

		if !result["root2"] {
			t.Error("Expected root2 to be required")
		}
	})

	t.Run("Enables dependent sections based on option selection", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Env:     "FEATURE_TOGGLE",
				Options: []PromptOption{
					{
						Name:     "Enable",
						Value:    "yes",
						Requires: "feature-config",
					},
				},
			},
		}

		rootSections := []string{"root"}
		effectiveValues := map[string]string{
			"FEATURE_TOGGLE": "yes",
		}

		result := determineRequiredSections(prompts, rootSections, effectiveValues)

		if !result["root"] {
			t.Error("Expected root to be required")
		}

		if !result["feature-config"] {
			t.Error("Expected feature-config to be enabled")
		}
	})

	t.Run("Does not enable sections for unselected options", func(t *testing.T) {
		prompts := []UserPrompt{
			{
				Section: "root",
				Env:     "FEATURE_TOGGLE",
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
		}

		rootSections := []string{"root"}
		effectiveValues := map[string]string{
			"FEATURE_TOGGLE": "yes",
		}

		result := determineRequiredSections(prompts, rootSections, effectiveValues)

		if !result["feature-config"] {
			t.Error("Expected feature-config to be enabled")
		}

		if result["disable-config"] {
			t.Error("Expected disable-config to not be enabled")
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
