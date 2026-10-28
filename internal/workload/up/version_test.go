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

package up

import (
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecMatches_TypeMismatchIsNoMatchNotError(t *testing.T) {
	loaded := loadedFrom(`{"artifact": {"name": "x", "type": "agent", "spec": {}}}`)
	doc := workload.Document{"type": "service", "spec": map[string]any{}}

	matches, err := specMatches(loaded, doc)

	require.NoError(t, err, "a type mismatch is an ordinary no-match, not a fault")
	assert.False(t, matches)
}

func TestSpecMatches_TypeComparisonFoldsCaseAndDefaultsLiveSide(t *testing.T) {
	// A file saying "Service" against a live doc that states no type at all
	// must match: the platform defaults an untyped artifact to service, and
	// its enums disagree about casing.
	loaded := loadedFrom(`{"artifact": {"name": "x", "type": "Service", "spec": {}}}`)
	doc := workload.Document{"spec": map[string]any{}}

	matches, err := specMatches(loaded, doc)

	require.NoError(t, err)
	assert.True(t, matches)
}

func TestSpecMatches_SpecDifferenceIsNoMatch(t *testing.T) {
	loaded := loadedFrom(`{"artifact": {"name": "x", "type": "service", "spec": {"port": 8080}}}`)
	doc := workload.Document{"type": "service", "spec": map[string]any{"port": float64(9090)}}

	matches, err := specMatches(loaded, doc)

	require.NoError(t, err)
	assert.False(t, matches)
}
