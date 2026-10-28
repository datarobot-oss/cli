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

package countflags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositiveInt_AppliesDefault(t *testing.T) {
	cmd := &cobra.Command{}

	var limit int

	cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

	require.NoError(t, cmd.ParseFlags(nil))
	assert.Equal(t, 100, limit)
}

func TestPositiveInt_ParsesValidValues(t *testing.T) {
	cmd := &cobra.Command{}

	var limit int

	cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

	require.NoError(t, cmd.ParseFlags([]string{"--limit", "10"}))
	assert.Equal(t, 10, limit)
}

func TestPositiveInt_RejectsNonPositiveValues(t *testing.T) {
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "-1"}} {
		cmd := &cobra.Command{}

		var limit int

		cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

		// A page size of 0 would fetch nothing and read as an empty result
		// rather than an error, so it is rejected at parse time.
		err := cmd.ParseFlags(args)
		require.Errorf(t, err, "args %v", args)
		assert.Contains(t, err.Error(), "must be a positive integer")
	}
}

func TestPositiveInt_RejectsGarbage(t *testing.T) {
	cmd := &cobra.Command{}

	var limit int

	cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

	// Non-numeric input keeps pflag's underlying strconv error.
	err := cmd.ParseFlags([]string{"--limit", "abc"})
	require.Error(t, err)
}

func TestPositiveInt_RejectsOutOfRangeValues(t *testing.T) {
	cmd := &cobra.Command{}

	var limit int

	cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

	// Beyond the int64 range strconv.ParseInt itself reports "value out of
	// range"; nothing is truncated or silently clamped.
	err := cmd.ParseFlags([]string{"--limit", "99999999999999999999"})
	require.Error(t, err)
}

func TestNonNegativeInt_AllowsZero(t *testing.T) {
	cmd := &cobra.Command{}

	var offset int

	cmd.Flags().Var(NonNegativeInt(&offset, 0), "offset", "usage")

	// Zero is meaningful for offset-style flags ("from the start"), unlike
	// page sizes, so only negatives are rejected.
	require.NoError(t, cmd.ParseFlags([]string{"--offset", "0"}))
	assert.Equal(t, 0, offset)
}

func TestNonNegativeInt_RejectsNegativeValues(t *testing.T) {
	cmd := &cobra.Command{}

	var offset int

	cmd.Flags().Var(NonNegativeInt(&offset, 0), "offset", "usage")

	err := cmd.ParseFlags([]string{"--offset", "-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a non-negative integer")
}

// TestGetIntRoundTrip pins the contract telemetry closures rely on: the
// custom value reports type "int", so Flags().GetInt converts via String()
// instead of failing with a flag-type mismatch.
func TestGetIntRoundTrip(t *testing.T) {
	cmd := &cobra.Command{}

	var limit int

	cmd.Flags().Var(PositiveInt(&limit, 100), "limit", "usage")

	require.NoError(t, cmd.ParseFlags([]string{"--limit", "7"}))

	got, err := cmd.Flags().GetInt("limit")
	require.NoError(t, err)
	assert.Equal(t, 7, got)
}
