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
		expected := "#\n# Lorem Ipsum.\nMY_VAR=\"my-value\""

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
		expected := "#\n# Lorem Ipsum.\n# More info here.\nMY_VAR=\"my-value\""

		if str != expected {
			t.Errorf("Expected '%s', got '%s'", expected, str)
		}
	})
}
