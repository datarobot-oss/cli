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

import "github.com/datarobot/cli/internal/drapi/filesapi"

// FileEntry is the minimal per-file shape needed by Diff.
type FileEntry struct {
	Hash string
	Size int64
}

type (
	LocalManifest  = map[string]FileEntry
	RemoteManifest = map[string]FileEntry
	BaseManifest   = map[string]FileEntry
)

// Diff produces a SyncPlan from the three input manifests. The result is
// returned unsorted; callers must call Sort before display or execution.
func Diff(base, local, remote BaseManifest) *SyncPlan {
	plan := &SyncPlan{}

	for path := range pathUnion(base, local, remote) {
		b := base[path]
		l := local[path]
		r := remote[path]

		cls := Classify(b.Hash, l.Hash, r.Hash)
		act := ActionFor(cls)

		if act == ActSkip {
			continue
		}

		plan.Append(FileAction{
			Path:           path,
			Classification: cls,
			Action:         act,
			LocalSize:      l.Size,
			RemoteSize:     r.Size,
			LocalHash:      l.Hash,
			RemoteHash:     r.Hash,
		})
	}

	return plan
}

func pathUnion(maps ...BaseManifest) map[string]struct{} {
	out := make(map[string]struct{})

	for _, m := range maps {
		for k := range m {
			out[k] = struct{}{}
		}
	}

	return out
}

// FromFilesAPI converts a filesapi-shaped manifest into the diff shape.
func FromFilesAPI(remote map[string]filesapi.FileMeta) RemoteManifest {
	out := make(RemoteManifest, len(remote))
	for k, v := range remote {
		out[k] = FileEntry{Hash: v.Hash, Size: v.Size}
	}

	return out
}
