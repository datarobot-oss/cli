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

package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibleCLIVersion(t *testing.T) {
	tests := []struct {
		name       string
		cliVersion string
		constraint string
		compatible bool
		expectErr  bool
	}{
		// No constraint - always compatible
		{
			name:       "empty constraint is always compatible",
			cliVersion: "1.0.0",
			constraint: "",
			compatible: true,
		},
		// Dev version - always compatible
		{
			name:       "dev CLI version is always compatible",
			cliVersion: "dev",
			constraint: ">=1.0.0",
			compatible: true,
		},
		// Latest CLI + plugin with no CLI constraints
		{
			name:       "latest CLI with no constraints",
			cliVersion: "2.0.0",
			constraint: "",
			compatible: true,
		},
		// Latest CLI + plugin with CLI min version (using >= constraint)
		{
			name:       "latest CLI satisfies min version constraint",
			cliVersion: "2.0.0",
			constraint: ">=1.0.0",
			compatible: true,
		},
		// Latest CLI + plugin with CLI max version (using < constraint)
		{
			name:       "latest CLI within max version constraint",
			cliVersion: "1.5.0",
			constraint: "< 2.0.0",
			compatible: true,
		},
		{
			name:       "latest CLI exceeds max version constraint",
			cliVersion: "2.0.0",
			constraint: "< 2.0.0",
			compatible: false,
		},
		// Old CLI + plugin with no CLI constraints
		{
			name:       "old CLI with no constraints",
			cliVersion: "0.1.0",
			constraint: "",
			compatible: true,
		},
		// Old CLI + plugin with CLI min version → incompatible
		{
			name:       "old CLI below min version constraint",
			cliVersion: "0.1.0",
			constraint: ">=1.0.0",
			compatible: false,
		},
		// Old CLI + plugin with CLI max version
		{
			name:       "old CLI within max version constraint",
			cliVersion: "0.1.0",
			constraint: "< 2.0.0",
			compatible: true,
		},
		// Semver constraint patterns
		{
			name:       "caret constraint compatible",
			cliVersion: "1.5.0",
			constraint: "^1.0.0",
			compatible: true,
		},
		{
			name:       "caret constraint incompatible major bump",
			cliVersion: "2.0.0",
			constraint: "^1.0.0",
			compatible: false,
		},
		{
			name:       "tilde constraint compatible",
			cliVersion: "1.0.5",
			constraint: "~1.0.0",
			compatible: true,
		},
		{
			name:       "tilde constraint incompatible minor bump",
			cliVersion: "1.1.0",
			constraint: "~1.0.0",
			compatible: false,
		},
		{
			name:       "exact version match",
			cliVersion: "1.2.3",
			constraint: "1.2.3",
			compatible: true,
		},
		{
			name:       "exact version mismatch",
			cliVersion: "1.2.4",
			constraint: "1.2.3",
			compatible: false,
		},
		{
			name:       "range constraint compatible",
			cliVersion: "1.5.0",
			constraint: ">= 1.0.0, < 2.0.0",
			compatible: true,
		},
		{
			name:       "range constraint incompatible",
			cliVersion: "2.0.0",
			constraint: ">= 1.0.0, < 2.0.0",
			compatible: false,
		},
		// Error cases
		{
			name:       "invalid constraint syntax",
			cliVersion: "1.0.0",
			constraint: "@invalid",
			compatible: false,
			expectErr:  true,
		},
		{
			name:       "invalid CLI version",
			cliVersion: "not-semver",
			constraint: ">=1.0.0",
			compatible: false,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compatibleCLIVersion(tt.cliVersion, tt.constraint)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.compatible, result)
		})
	}
}
