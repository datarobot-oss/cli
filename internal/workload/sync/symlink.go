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

// SkippedSymlink describes a symlink the walk did not follow, collected so
// the display layer can tell the user a file they expect on the remote is
// absent. IsDir distinguishes a single skipped file from an entire omitted
// subtree (a directory symlink prunes all of its children), which is the
// distinction that makes the notice actionable.
type SkippedSymlink struct {
	Path  string
	IsDir bool
}
