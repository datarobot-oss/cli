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

package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseID_TrimsAValidID(t *testing.T) {
	id, err := ParseID("  68b0aa11bb22cc33dd44ee55  ")
	require.NoError(t, err)
	assert.Equal(t, ID("68b0aa11bb22cc33dd44ee55"), id)
}

func TestParseID_Rejects(t *testing.T) {
	for name, raw := range map[string]string{
		"blank":        "   ",
		"too short":    "68b0aa11",
		"too long":     "68b0aa11bb22cc33dd44ee5566",
		"not hex":      "68b0aa11bb22cc33dd44eezz",
		"inner spaces": "68b0aa11bb22 cc33dd44ee55",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseID(raw)
			require.Error(t, err)
		})
	}
}
