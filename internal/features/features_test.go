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

package features

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

type mockProvider struct {
	enabledFeatures map[string]bool
}

func (m *mockProvider) IsEnabled(name string) bool {
	return m.enabledFeatures[name]
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name         string
		featureName  string
		envVarValue  string
		expectedBool bool
	}{
		{
			name:         "env var true",
			featureName:  "test",
			envVarValue:  "true",
			expectedBool: true,
		},
		{
			name:         "env var 1",
			featureName:  "test",
			envVarValue:  "1",
			expectedBool: true,
		},
		{
			name:         "env var false",
			featureName:  "test",
			envVarValue:  "false",
			expectedBool: false,
		},
		{
			name:         "env var 0",
			featureName:  "test",
			envVarValue:  "0",
			expectedBool: false,
		},
		{
			name:         "env var not set",
			featureName:  "test",
			envVarValue:  "",
			expectedBool: false,
		},
		{
			name:         "hyphenated feature name",
			featureName:  "my-feature",
			envVarValue:  "true",
			expectedBool: true,
		},
	}

	// Save original provider and restore after test
	originalProvider := provider

	defer func() { SetProvider(originalProvider) }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute the env key (hyphens become underscores)
			envKey := "DATAROBOT_CLI_FEATURE_" + strings.ToUpper(strings.ReplaceAll(tt.featureName, "-", "_"))

			if tt.envVarValue != "" {
				t.Setenv(envKey, tt.envVarValue)
			}

			result := Enabled(tt.featureName)
			assert.Equal(t, tt.expectedBool, result)
		})
	}
}

func TestSetProvider(t *testing.T) {
	originalProvider := provider

	defer func() { SetProvider(originalProvider) }()

	// Set up mock provider
	mockProv := &mockProvider{
		enabledFeatures: map[string]bool{
			"enabled-feature":  true,
			"disabled-feature": false,
		},
	}
	SetProvider(mockProv)

	// Test that Enabled uses the new provider
	assert.True(t, Enabled("enabled-feature"))
	assert.False(t, Enabled("disabled-feature"))
	assert.False(t, Enabled("unknown-feature"))
}

func TestSetProviderNil(t *testing.T) {
	originalProvider := provider

	defer func() { SetProvider(originalProvider) }()

	// SetProvider should ignore nil values
	SetProvider(nil)

	// Verify the original provider is still in use
	assert.Same(t, originalProvider, provider)
}

func TestSetGate(t *testing.T) {
	tests := []struct {
		name             string
		cmd              *cobra.Command
		featureName      string
		existingAnnot    map[string]string
		expectedGate     string
		shouldPreserveAn bool
	}{
		{
			name:         "adds gate to command with no annotations",
			cmd:          &cobra.Command{Use: "test"},
			featureName:  "my-feature",
			expectedGate: "my-feature",
		},
		{
			name:             "preserves existing annotations",
			cmd:              &cobra.Command{Use: "test", Annotations: map[string]string{"key": "value"}},
			featureName:      "my-feature",
			expectedGate:     "my-feature",
			shouldPreserveAn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGate(tt.cmd, tt.featureName)

			assert.Equal(t, tt.expectedGate, tt.cmd.Annotations[AnnotationKey])

			if tt.shouldPreserveAn {
				assert.Equal(t, "value", tt.cmd.Annotations["key"])
			}
		})
	}
}
