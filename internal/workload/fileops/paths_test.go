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

package fileops

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "POSIXAlready", in: "models/bert/weights.bin", want: "models/bert/weights.bin"},
		{name: "LeadingDotSlash", in: "./agent.py", want: "agent.py"},
		{name: "DoubleLeadingDotSlash", in: "././config.yaml", want: "config.yaml"},
		{name: "TrailingSlash", in: "models/", want: "models"},
		// macOS NFD form of "café": e + combining acute (́) → NFC: "café".
		{name: "NFDtoNFC", in: "café.py", want: "café.py"},
		{name: "Empty", in: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizePath(tc.in))
		})
	}

	// filepath.ToSlash is platform-conditional (no-op on Unix where '\' is
	// a legal filename byte). Only assert backslash conversion on Windows
	// where the OS-native walker hands us '\'-separated paths.
	if runtime.GOOS == "windows" {
		t.Run("WindowsBackslashes", func(t *testing.T) {
			assert.Equal(t, "models/bert/weights.bin", NormalizePath(`models\bert\weights.bin`))
		})
	}
}

func TestSafeRelPath(t *testing.T) {
	t.Run("Accept", func(t *testing.T) {
		good := []string{
			"agent.py",
			"models/bert/weights.bin",
			"src/sub/.config",
			"a/b/../b/c", // .. cancels inside the tree, net path stays under root
			"foo..bar",   // .. only matters as a path segment
			"...hidden",
		}

		for _, p := range good {
			require.NoError(t, SafeRelPath(p), "expected accept for %q", p)
		}
	})

	t.Run("RejectEmpty", func(t *testing.T) {
		assert.ErrorContains(t, SafeRelPath(""), "empty")
	})

	t.Run("RejectAbsolute", func(t *testing.T) {
		assert.ErrorContains(t, SafeRelPath("/etc/passwd"), "absolute")
	})

	t.Run("RejectEscape", func(t *testing.T) {
		bad := []string{
			"..",
			"../",
			"../etc/passwd",
			"../../foo",
			"a/../../b",
		}

		for _, p := range bad {
			err := SafeRelPath(p)
			require.Error(t, err, "expected reject for %q", p)
			assert.Contains(t, err.Error(), "escapes")
		}
	})

	// Backslash inputs slip past path.Clean on Unix dev boxes (`\` is a
	// legal filename byte there), but `filepath.Join` interprets them as
	// separators on Windows — so a remote sending `..\escape` would
	// escape the project root downstream. Reject at the boundary.
	t.Run("RejectBackslash", func(t *testing.T) {
		bad := []string{
			`..\escape`,
			`foo\bar`,
			`..\..\etc`,
			`a\..\..\b`,
		}

		for _, p := range bad {
			err := SafeRelPath(p)
			require.Error(t, err, "expected reject for %q", p)
			assert.Contains(t, err.Error(), "backslash")
		}
	})
}

func TestDetectCaseCollisions(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []CaseCollision
	}{
		{
			name: "NoCollisions",
			in:   []string{"agent.py", "utils/helper.py"},
			want: nil,
		},
		{
			name: "SingleCollision",
			in:   []string{"Config.yaml", "config.yaml"},
			want: []CaseCollision{{Lowered: "config.yaml", Paths: []string{"Config.yaml", "config.yaml"}}},
		},
		{
			name: "MultiCollision",
			in:   []string{"a.py", "A.py", "b.py", "B.py"},
			want: []CaseCollision{
				{Lowered: "a.py", Paths: []string{"A.py", "a.py"}},
				{Lowered: "b.py", Paths: []string{"B.py", "b.py"}},
			},
		},
		{
			name: "NestedSamePath",
			in:   []string{"src/Foo.py", "src/foo.py"},
			want: []CaseCollision{{Lowered: "src/foo.py", Paths: []string{"src/Foo.py", "src/foo.py"}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := make(map[string]struct{}, len(tc.in))
			for _, p := range tc.in {
				set[p] = struct{}{}
			}

			got := DetectCaseCollisions(set)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFormatCaseCollisions(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		assert.Empty(t, FormatCaseCollisions(nil))
	})

	t.Run("Format", func(t *testing.T) {
		out := FormatCaseCollisions([]CaseCollision{
			{Lowered: "config.yaml", Paths: []string{"Config.yaml", "config.yaml"}},
		})
		assert.Contains(t, out, "case-only path collisions")
		assert.Contains(t, out, "Config.yaml vs config.yaml")
	})
}
