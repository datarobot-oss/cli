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

package paginate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// row is a stand-in for whatever row type a caller's envelope decodes into.
type row struct {
	ID string
}

func TestWalk_AccumulatesAcrossPages(t *testing.T) {
	fetch := func(pageURL string) ([]row, string, error) {
		switch pageURL {
		case "first":
			return []row{{"a"}, {"b"}}, "second", nil
		case "second":
			return []row{{"c"}}, "", nil
		default:
			t.Fatalf("unexpected page %q", pageURL)

			return nil, "", nil
		}
	}

	rows, err := Walk("first", 10, fetch)
	require.NoError(t, err)
	assert.Equal(t, []row{{"a"}, {"b"}, {"c"}}, rows)
}

func TestWalk_StopsAtLimitAndTruncates(t *testing.T) {
	calls := 0

	fetch := func(pageURL string) ([]row, string, error) {
		calls++

		if pageURL == "first" {
			return []row{{"a"}, {"b"}, {"c"}}, "second", nil
		}

		return []row{{"d"}, {"e"}, {"f"}}, "", nil
	}

	rows, err := Walk("first", 4, fetch)
	require.NoError(t, err)

	// Exactly limit rows come back even though the last page overshot, and
	// no page beyond the one that satisfies the limit is fetched.
	assert.Equal(t, []row{{"a"}, {"b"}, {"c"}, {"d"}}, rows)
	assert.Equal(t, 2, calls)
}

func TestWalk_SinglePageNeedsNoExtraFetch(t *testing.T) {
	calls := 0

	fetch := func(pageURL string) ([]row, string, error) {
		calls++

		return []row{{"a"}}, "", nil
	}

	rows, err := Walk("first", 5, fetch)
	require.NoError(t, err)
	assert.Equal(t, []row{{"a"}}, rows)
	assert.Equal(t, 1, calls)
}

func TestWalk_PropagatesFetchErrors(t *testing.T) {
	boom := errors.New("boom")

	fetch := func(pageURL string) ([]row, string, error) {
		return nil, "", boom
	}

	_, err := Walk("first", 5, fetch)
	require.ErrorIs(t, err, boom)
}

func TestWalk_RejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		fetch := func(pageURL string) ([]row, string, error) {
			t.Fatalf("fetch must not run for limit %d", limit)

			return nil, "", nil
		}

		_, err := Walk("first", limit, fetch)
		require.Errorf(t, err, "limit %d", limit)
		assert.Contains(t, err.Error(), "limit must be positive")
	}
}
