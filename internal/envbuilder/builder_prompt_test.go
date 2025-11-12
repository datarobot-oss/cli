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
		prompt := UserPrompt{
			Env:    "MY_VAR",
			Key:    "my-key",
			Value:  "my-value",
			Active: true,
		}

		str := prompt.String()

		if str != "MY_VAR=my-value" {
			t.Errorf("Expected 'MY_VAR=my-value', got '%s'", str)
		}
	})

	t.Run("Returns commented Key when Env is empty", func(t *testing.T) {
		prompt := UserPrompt{
			Key:   "my-key",
			Value: "my-value",
		}

		str := prompt.String()

		if str != "# my-key=my-value" {
			t.Errorf("Expected '# my-key=my-value', got '%s'", str)
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

		if str != "\n# Lorem Ipsum.\nMY_VAR=my-value" {
			t.Errorf("Expected '\n# Lorem Ipsum.\nMY_VAR=my-value', got '%s'", str)
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

		if str != "\n# Lorem Ipsum.\n# More info here.\nMY_VAR=my-value" {
			t.Errorf("Expected '\n# Lorem Ipsum.\n# More info here.\nMY_VAR=my-value', got '%s'", str)
		}
	})
}
