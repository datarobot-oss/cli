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
	"sort"
	"strings"

	"github.com/datarobot/cli/internal/workload/wapi"
)

// executableNoticeMaxNames caps the file list so a tree of scripts stays one line.
const executableNoticeMaxNames = 5

// uploadExecutable is the bit an upload carries: the file's own, or on a host
// without one (Windows) the remote's, so an edit there does not clear it.
// Nil when neither side knows; the upload then goes as a plain file.
func uploadExecutable(local, remote *bool) *bool {
	if local != nil {
		return local
	}

	return remote
}

func isSet(b *bool) bool {
	return b != nil && *b
}

// hasUnknownExecutable reports a base entry written before the bit was tracked.
func hasUnknownExecutable(base BaseManifest) bool {
	for _, entry := range base {
		if entry.Executable == nil {
			return true
		}
	}

	return false
}

// persistExecutableBackfill records the bits a backfill listing learned for
// files whose bytes still match, so the next sync takes the fast path again. A
// run that executes a plan writes them in its state phase instead.
func persistExecutableBackfill(e *Engine) error {
	if !e.execBackfill {
		return nil
	}

	m, err := wapi.LoadManifest(e.projectDir)
	if err != nil {
		return err
	}

	changed := false

	for path, meta := range m.Files {
		r, ok := e.remote[path]
		if meta.Executable != nil || !ok || r.Executable == nil || r.Hash != meta.Hash {
			continue
		}

		meta.Executable = r.Executable
		m.Files[path] = meta
		changed = true
	}

	if !changed {
		return nil
	}

	return wapi.SaveManifest(e.projectDir, m)
}

// executableNotice warns that the image build drops the bit the catalog now
// keeps, naming the executable files whose bit this sync brought to the
// catalog, so an edit to a file it already named stays quiet; "" when there
// are none. Remove it once image builds apply the bit.
func executableNotice(sent map[string]FileEntry, before RemoteManifest) string {
	var names []string

	for path, entry := range sent {
		if isSet(entry.Executable) && !isSet(before[path].Executable) {
			names = append(names, path)
		}
	}

	if len(names) == 0 {
		return ""
	}

	sort.Strings(names)

	listed := names
	if len(listed) > executableNoticeMaxNames {
		listed = listed[:executableNoticeMaxNames]
	}

	list := strings.Join(listed, ", ")
	if extra := len(names) - len(listed); extra > 0 {
		list += fmt.Sprintf(" and %d more", extra)
	}

	return "Image builds do not keep the executable bit yet; use `COPY --chmod=755` in your Dockerfile for: " + list
}
