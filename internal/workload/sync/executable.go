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

// executableNotice warns that the image build drops the bit the catalog now
// keeps, naming the executable files this sync uploaded; "" when there are none.
func executableNotice(sent map[string]FileEntry) string {
	var names []string

	for path, entry := range sent {
		if isSet(entry.Executable) {
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
