// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"testing"
)

func TestPromptString(t *testing.T) {
	t.Run("Returns Env when present", func(t *testing.T) {
		tests := []struct {
			prompt   UserPrompt
			expected string
		}{
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my-value`, Active: true},
				expected: `MY_VAR="my-value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my value`, Active: true},
				expected: `MY_VAR="my value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my"value`, Active: true},
				expected: `MY_VAR="my\"value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `"my-value`, Active: true},
				expected: `MY_VAR="\"my-value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my-value"`, Active: true},
				expected: `MY_VAR="my-value\""`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my' value`, Active: true},
				expected: `MY_VAR="my' value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `'my-value`, Active: true},
				expected: `MY_VAR="'my-value"`,
			},
			{
				prompt:   UserPrompt{Env: "MY_VAR", Key: "my-key", Value: `my-value'`, Active: true},
				expected: `MY_VAR="my-value'"`,
			},
		}

		for _, test := range tests {
			result := test.prompt.String()

			if result != test.expected {
				t.Errorf("Expected '%s', got '%s'", test.expected, result)
			}
		}
	})

	t.Run("Returns commented Key when Env is empty", func(t *testing.T) {
		prompt := UserPrompt{
			Key:   "my-key",
			Value: "my-value",
		}

		str := prompt.String()
		expected := `# my-key="my-value"`

		if str != expected {
			t.Errorf("Expected '%s', got '%s'", expected, str)
		}
	})

	t.Run("Returns help as comment above env var when help is present", func(t *testing.T) {
		prompt := UserPrompt{
			Env:    "MY_VAR",
			Key:    "my-key",
			Value:  "my-value",
			Active: true,
			Help:   "Lorem Ipsum.",
		}

		str := prompt.String()
		expected := "\n# Lorem Ipsum.\nMY_VAR=\"my-value\""

		if str != expected {
			t.Errorf("Expected '%s', got '%s'", expected, str)
		}
	})

	t.Run("Returns multiline comment when multiline help is present", func(t *testing.T) {
		prompt := UserPrompt{
			Env:    "MY_VAR",
			Key:    "my-key",
			Value:  "my-value",
			Active: true,
			Help:   "Lorem Ipsum.\nMore info here.",
		}

		str := prompt.String()
		expected := "\n# Lorem Ipsum.\n# More info here.\nMY_VAR=\"my-value\""

		if str != expected {
			t.Errorf("Expected '%s', got '%s'", expected, str)
		}
	})
}
