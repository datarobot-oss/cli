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

package format

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes_Range(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"sub-unit", 512, "512 B"},
		{"unit-edge", 1023, "1023 B"},
		{"one-kb", 1024, "1.0 KB"},
		{"one-and-half-kb", 1024 + 512, "1.5 KB"},
		{"one-mb", 1024 * 1024, "1.0 MB"},
		{"one-gb", 1024 * 1024 * 1024, "1.0 GB"},
		{"one-tb", int64(1024) * 1024 * 1024 * 1024, "1.0 TB"},
		{"one-pb", int64(1024) * 1024 * 1024 * 1024 * 1024, "1.0 PB"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, Bytes(tc.in))
		})
	}
}

// Regression: prior implementation indexed past the suffix slice for n >= 1024^5,
// causing an index-out-of-range panic. The capped loop holds at PB.
func TestBytes_NoPanicAtExtreme(t *testing.T) {
	t.Parallel()

	got := Bytes(int64(1) << 60)
	assert.True(t, strings.HasSuffix(got, " PB"), "expected PB suffix, got %q", got)

	got = Bytes(math.MaxInt64)
	assert.True(t, strings.HasSuffix(got, " PB"), "expected PB suffix at MaxInt64, got %q", got)
}
