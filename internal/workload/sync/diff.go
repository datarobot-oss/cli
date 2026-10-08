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
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload/ignore"
)

// FileEntry is the minimal per-file shape needed by Diff. Executable is nil
// when the bit is unknown: an old manifest entry, an old server, or Windows.
type FileEntry struct {
	Hash       string
	Size       int64
	Executable *bool
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

		cls := refineExecutable(Classify(b.Hash, l.Hash, r.Hash), b, l, r)

		// A system-excluded path is never on the local side, since the walk
		// does not list it, so a remote copy is an older CLI's upload from
		// before the excludes reached every depth. Read as it stands, that
		// copy is REMOTE_ADDED on a machine with no base, or EDIT_DEL_CONFLICT
		// when a teammate touched it, and either downloads over the real
		// local file (a vendored checkout's .git/HEAD). It is a remote delete
		// instead: the local file is left alone, and the remote is brought
		// in line with what the walk uploads.
		if r.Hash != "" && ignore.IsSystemExcluded(path) {
			cls = ClsLocalDeleted
		}

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
			LocalExec:      l.Executable,
			RemoteExec:     r.Executable,
		})
	}

	return plan
}

// refineExecutable turns a path whose content agrees on both sides into a
// modification when only its executable bit moved since the last sync.
func refineExecutable(cls Classification, b, l, r FileEntry) Classification {
	if cls != ClsUnchanged && cls != ClsConverged {
		return cls
	}

	if exec := classifyExecutable(b.Executable, l.Executable, r.Executable); exec != ClsUnchanged {
		return exec
	}

	return cls
}

// classifyExecutable is the three-way diff of the executable bit alone. An
// unknown side is never drift: without a base there is no telling who moved.
func classifyExecutable(base, local, remote *bool) Classification {
	if base == nil {
		return ClsUnchanged
	}

	localChanged := local != nil && *local != *base
	remoteChanged := remote != nil && *remote != *base

	switch {
	case localChanged && !remoteChanged:
		return ClsLocalModified
	case remoteChanged && !localChanged:
		return ClsRemoteModified
	}

	// Both moved means both agree, since the bit has two values.
	return ClsUnchanged
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
		out[k] = FileEntry{Hash: v.Hash, Size: v.Size, Executable: v.Executable}
	}

	return out
}
