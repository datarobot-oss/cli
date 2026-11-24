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

package platform

import (
	"strings"
	"unicode"
)

// textLimit caps one server-supplied string, in runes.
const textLimit = 512

// clean makes a string from the install safe for a terminal and for a reader
// that takes text as instructions. It drops control characters (C0, C1 and
// ESC), format characters such as bidi overrides, zero width and tag
// characters, private use and surrogate code points, and invalid UTF-8. Line
// and paragraph separators and every other kind of white space become one
// space, so the result is one line. It cuts the result at textLimit runes.
func clean(s string) string {
	spaced := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs):
			return -1
		}

		return r
	}, strings.ToValidUTF8(s, ""))

	out := strings.Join(strings.Fields(spaced), " ")

	if runes := []rune(out); len(runes) > textLimit {
		return strings.TrimSpace(string(runes[:textLimit]))
	}

	return out
}

// cleanAll returns a cleaned copy of items, never nil.
func cleanAll(items []string) []string {
	out := make([]string, 0, len(items))

	for _, item := range items {
		out = append(out, clean(item))
	}

	return out
}
