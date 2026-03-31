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

package tools

import (
	"testing"

	"github.com/datarobot/cli/internal/version"
	"github.com/stretchr/testify/assert"
)

func TestSufficientVersionTrue(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "3.5.7", minimal: "3.5.7"},
		{installed: "3.5.9", minimal: "3.5.7"},
		{installed: "3.7.6", minimal: "3.5.7"},
		{installed: "5.4.6", minimal: "3.5.7"},
		// Git-describe format with pre-release metadata
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.54"},
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.55"},
		// Fallback format for fresh clones
		{installed: "v0.0.0-dev.42.gabcd1234", minimal: "0.0.0"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != true {
			t.Errorf("for installed %s and minimal %s, expected sufficient", testCase.installed, testCase.minimal)
		}
	}
}

func TestSufficientVersionFalse(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "2.6.8", minimal: "3.5.7"},
		{installed: "3.4.8", minimal: "3.5.7"},
		{installed: "3.5.6", minimal: "3.5.7"},
		// Git-describe format with pre-release metadata
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.56"},
		// Fallback format is insufficient for higher minimal
		{installed: "v0.0.0-dev.42.gabcd1234", minimal: "0.1.0"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != false {
			t.Errorf("for installed %s and minimal %s, expected insufficient", testCase.installed, testCase.minimal)
		}
	}
}

func TestSufficientSelfVersion(t *testing.T) {
	tests := []struct {
		name           string
		versionValue   string
		minimalVersion string
		expected       bool
	}{
		{
			name:           "dev always returns true",
			versionValue:   "dev",
			minimalVersion: "99.99.99",
			expected:       true,
		},
		{
			name:           "git-describe format sufficient",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.54",
			expected:       true,
		},
		{
			name:           "git-describe format exactly matching",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.55",
			expected:       true,
		},
		{
			name:           "git-describe format insufficient",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.56",
			expected:       false,
		},
		{
			name:           "fallback dev format sufficient",
			versionValue:   "v0.0.0-dev.42.gabcd1234",
			minimalVersion: "0.0.0",
			expected:       true,
		},
		{
			name:           "empty minimal returns false",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalVersion := version.Version

			defer func() { version.Version = originalVersion }()

			version.Version = tt.versionValue
			result := SufficientSelfVersion(tt.minimalVersion)
			assert.Equal(t, tt.expected, result, "version=%s minimal=%s", tt.versionValue, tt.minimalVersion)
		})
	}
}
