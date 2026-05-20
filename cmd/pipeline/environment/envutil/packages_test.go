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

package envutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePackages_RepeatedFlags(t *testing.T) {
	got, err := NormalizePackages([]string{"numpy", "pandas==2.0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"numpy", "pandas==2.0"}, got)
}

func TestNormalizePackages_CommaSeparated(t *testing.T) {
	got, err := NormalizePackages([]string{"numpy,pandas==2.0, scikit-learn"})
	require.NoError(t, err)
	assert.Equal(t, []string{"numpy", "pandas==2.0", "scikit-learn"}, got)
}

func TestNormalizePackages_DropsBlanks(t *testing.T) {
	got, err := NormalizePackages([]string{"numpy,,", "  ,pandas"})
	require.NoError(t, err)
	assert.Equal(t, []string{"numpy", "pandas"}, got)
}

func TestNormalizePackages_RejectsEmpty(t *testing.T) {
	_, err := NormalizePackages(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one package")

	_, err = NormalizePackages([]string{"", "  ,"})
	require.Error(t, err)
}
