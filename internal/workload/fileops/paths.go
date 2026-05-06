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
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SafeRelPath rejects paths that aren't safe to filepath.Join with a
// project root: empty, absolute, containing a backslash, or escaping via
// "..". Defends against hostile or buggy server responses. Wire-format
// paths are POSIX by contract; a backslash either means a Windows-style
// traversal attempt or an OS-native separator that path.Clean below
// won't catch — reject loud at the boundary.
func SafeRelPath(p string) error {
	if p == "" {
		return errors.New("empty path")
	}

	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("backslash in path not allowed: %q", p)
	}

	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return fmt.Errorf("absolute path not allowed: %q", p)
	}

	cleaned := path.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path escapes project root: %q", p)
	}

	return nil
}

// NormalizePath returns a canonical manifest key: forward slashes, NFC
// Unicode, no leading "./", no trailing slash.
func NormalizePath(p string) string {
	p = filepath.ToSlash(p)
	p = norm.NFC.String(p)

	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}

	return strings.TrimRight(p, "/")
}

// CaseCollision groups paths that differ only in case — would silently
// overwrite each other on case-insensitive filesystems (macOS, Windows).
type CaseCollision struct {
	Lowered string
	Paths   []string
}

// DetectCaseCollisions returns groups of paths that collide under
// case-insensitive comparison, sorted for deterministic output.
func DetectCaseCollisions(paths map[string]struct{}) []CaseCollision {
	groups := make(map[string][]string, len(paths))

	for p := range paths {
		key := strings.ToLower(p)
		groups[key] = append(groups[key], p)
	}

	var collisions []CaseCollision

	for key, ps := range groups {
		if len(ps) < 2 {
			continue
		}

		sort.Strings(ps)
		collisions = append(collisions, CaseCollision{Lowered: key, Paths: ps})
	}

	sort.Slice(collisions, func(i, j int) bool {
		return collisions[i].Lowered < collisions[j].Lowered
	})

	return collisions
}

// FormatCaseCollisions renders collisions as a multi-line user message.
func FormatCaseCollisions(cs []CaseCollision) string {
	if len(cs) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("case-only path collisions detected (would silently overwrite on a case-insensitive filesystem):\n")

	for _, c := range cs {
		fmt.Fprintf(&b, "  - %s\n", strings.Join(c.Paths, " vs "))
	}

	return strings.TrimRight(b.String(), "\n")
}
