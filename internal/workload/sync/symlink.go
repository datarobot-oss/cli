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

import (
	"fmt"
	"slices"
	"strings"

	"github.com/datarobot/cli/internal/workload/fileops"
)

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
	// Reason is why the walk did not follow it.
	Reason fileops.LinkOutcome
}

// Notice renders one skipped symlink as a self-contained stderr
// line. The wording differs by kind so a reader can tell "lost one file"
// apart from "lost an entire subtree", and both say the symlink was NOT
// uploaded or synced — not merely that one was found — so the user
// understands their file is absent from the remote.
func (s SkippedSymlink) Notice() string {
	if s.IsDir {
		return fmt.Sprintf(
			"skipped symlink: %s was not synced (it is a directory symlink that %s; the entire subtree under it is omitted from the sync)",
			s.Path, s.Why())
	}

	return fmt.Sprintf(
		"skipped symlink: %s was not uploaded (it is a symlink that %s)",
		s.Path, s.Why())
}

// Why completes "it is a symlink that ...".
func (s SkippedSymlink) Why() string {
	switch s.Reason {
	case fileops.LinkDangling:
		return "points at nothing"
	case fileops.LinkOutside:
		return "points outside the project"
	case fileops.LinkTargetIgnored:
		return "points at a path the ignore rules exclude"
	case fileops.LinkLoop:
		return "points back into a directory that contains it"
	case fileops.LinkNotAFile:
		return "points at something that is neither a file nor a directory"
	case fileops.LinkFollowed, fileops.LinkUnreadable, fileops.LinkIgnored:
	}

	return "could not be resolved"
}

// underLink reports whether path is a link the walk followed or lies under
// one.
func underLink(path string, links []string) bool {
	return slices.ContainsFunc(links, func(link string) bool {
		return path == link || strings.HasPrefix(path, link+"/")
	})
}
