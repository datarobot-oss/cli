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

package tools

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldRuleValidate(t *testing.T) {
	tests := []struct {
		name      string
		rule      FieldRule
		fieldName string
		value     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "required field present",
			rule:      FieldRule{Required: true},
			fieldName: "name",
			value:     "Python",
			wantErr:   false,
		},
		{
			name:      "required field missing",
			rule:      FieldRule{Required: true},
			fieldName: "name",
			value:     "",
			wantErr:   true,
			errSubstr: "'name' is required",
		},
		{
			name:      "semver field valid",
			rule:      FieldRule{Format: formatSemver},
			fieldName: "minimum-version",
			value:     "3.9.0",
			wantErr:   false,
		},
		{
			name:      "semver field empty is allowed",
			rule:      FieldRule{Format: formatSemver},
			fieldName: "minimum-version",
			value:     "",
			wantErr:   false,
		},
		{
			name:      "semver field invalid",
			rule:      FieldRule{Format: formatSemver},
			fieldName: "minimum-version",
			value:     "not-a-version",
			wantErr:   true,
			errSubstr: "'minimum-version'",
		},
		{
			name:      "optional string field empty",
			rule:      FieldRule{},
			fieldName: "url",
			value:     "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.validate(tt.fieldName, tt.value)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInstallCommandsSchemaValidate(t *testing.T) {
	schema := InstallCommandsSchema{
		MacOS:   FieldRule{Required: true},
		Linux:   FieldRule{Required: true},
		Windows: FieldRule{Required: true},
	}

	t.Run("all platforms provided — no errors", func(t *testing.T) {
		ic := InstallCommands{MacOS: "brew install x", Linux: "apt install x", Windows: "choco install x"}

		errs := schema.validate("tool", ic)

		assert.Empty(t, errs)
	})

	t.Run("current OS platform missing — error returned", func(t *testing.T) {
		var ic InstallCommands

		switch runtime.GOOS {
		case "darwin":
			ic = InstallCommands{Linux: "apt install x", Windows: "choco install x"}
		case "linux":
			ic = InstallCommands{MacOS: "brew install x", Windows: "choco install x"}
		default:
			ic = InstallCommands{MacOS: "brew install x", Linux: "apt install x"}
		}

		errs := schema.validate("tool", ic)

		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0], "[tool]")
	})

	t.Run("only non-current OS platforms missing — no errors, warnings logged", func(t *testing.T) {
		var ic InstallCommands

		switch runtime.GOOS {
		case "darwin":
			ic = InstallCommands{MacOS: "brew install x"}
		case "linux":
			ic = InstallCommands{Linux: "apt install x"}
		default:
			ic = InstallCommands{Windows: "choco install x"}
		}

		errs := schema.validate("tool", ic)

		assert.Empty(t, errs)
	})
}

func TestYAMLSchemaValidate(t *testing.T) {
	tests := []struct {
		name      string
		input     versionsYaml
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid with all fields",
			input: versionsYaml{
				"python": {Name: "Python", MinimumVersion: "3.9.0", Command: "python3", URL: "https://python.org"},
			},
			wantErr: false,
		},
		{
			name: "valid with name and command",
			input: versionsYaml{
				"dr": {Name: "DataRobot CLI", Command: "dr"},
			},
			wantErr: false,
		},
		{
			name: "valid with name, command, and semver minimum-version",
			input: versionsYaml{
				"node": {Name: "Node.js", Command: "node", MinimumVersion: "18.0.0"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			input: versionsYaml{
				"python": {MinimumVersion: "3.9.0"},
			},
			wantErr:   true,
			errSubstr: "[python]",
		},
		{
			name: "invalid minimum-version",
			input: versionsYaml{
				"python": {Name: "Python", MinimumVersion: "not-a-version"},
			},
			wantErr:   true,
			errSubstr: "[python]",
		},
		{
			name: "multiple invalid entries",
			input: versionsYaml{
				"python": {MinimumVersion: "3.9.0"},
				"node":   {Name: "Node.js", MinimumVersion: "bad-version"},
			},
			wantErr:   true,
			errSubstr: "validation errors",
		},
		{
			name:    "empty map is valid",
			input:   versionsYaml{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := versionsYamlSchema.Validate(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
