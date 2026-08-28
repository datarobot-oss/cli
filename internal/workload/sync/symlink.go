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

package sync

import "fmt"

// SymlinkNoticeBound is the maximum number of skipped symlinks the stderr
// prose lists individually before summarizing the remainder as a count.
// The JSON field always lists every symlink. Fixed at 5 so workers and
// validators assert the same number rather than each choosing a bound.
const SymlinkNoticeBound = 5

// SkippedSymlink describes a symlink the walk did not follow, collected so
// the display layer can tell the user a file they expect on the remote is
// absent. IsDir distinguishes a single skipped file from an entire omitted
// subtree (a directory symlink prunes all of its children), which is the
// distinction that makes the notice actionable.
type SkippedSymlink struct {
	Path  string
	IsDir bool
}

// skippedSymlinkNotice renders one skipped symlink as a self-contained stderr
// line. The wording differs by kind so a reader can tell "lost one file"
// apart from "lost an entire subtree", and both say the symlink was NOT
// uploaded or synced — not merely that one was found — so the user
// understands their file is absent from the remote.
func skippedSymlinkNotice(s SkippedSymlink) string {
	if s.IsDir {
		return fmt.Sprintf(
			"skipped symlink: %s was not synced (it is a directory symlink; the entire subtree under it is omitted from the sync)",
			s.Path)
	}

	return fmt.Sprintf(
		"skipped symlink: %s was not uploaded (it is a symlink, not a regular file)",
		s.Path)
}
