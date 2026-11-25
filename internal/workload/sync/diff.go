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
	return diff(base, local, remote, nil, nil)
}

// diff is Diff for a tree in which the walk followed links. A path at or under
// one is only ever pushed: a download, a local delete or a conflict copy there
// would write through the link into what it points at. Reading it against
// REMOTE as its base makes the local copy win every disagreement.
//
// The paths in aliases are left out, for planAliases to add once the plan has
// settled what the files they alias will hold.
func diff(base, local, remote BaseManifest, linkedPaths []string, aliases map[string]string) *SyncPlan {
	plan := &SyncPlan{}

	for path := range pathUnion(base, local, remote) {
		if _, ok := aliases[path]; ok {
			continue
		}

		b := base[path]
		l := local[path]
		r := remote[path]

		if underLink(path, linkedPaths) {
			b = plan.linkedBase(path, b, l, r)
		}

		plan.add(path, b, l, r)
	}

	return plan
}

// planAliases plans each path in aliases, which reaches through a followed
// link the file listed under its own name, as a push of what that file holds
// once the plan has run. A pull of that file changes the alias too, so pushing
// the alias's bytes from before the pull would undo it.
func (p *SyncPlan) planAliases(aliases map[string]string, base, local, remote BaseManifest) {
	if len(aliases) == 0 {
		return
	}

	pulled := p.pulled(remote)

	for alias, own := range aliases {
		l, ok := pulled[own]
		if !ok {
			l = local[own]
		}

		b, r := base[alias], remote[alias]

		p.add(alias, p.linkedBase(alias, b, l, r), l, r)
	}
}

// pulled is what each path the plan writes locally holds once it has run; a
// zero entry for a path it removes.
func (p *SyncPlan) pulled(remote BaseManifest) map[string]FileEntry {
	out := map[string]FileEntry{}

	for _, fa := range p.Downloads {
		out[fa.Path] = remote[fa.Path]
	}

	// Remote wins a conflict, so it leaves the remote copy, or nothing.
	for _, fa := range p.Conflicts {
		out[fa.Path] = remote[fa.Path]
	}

	for _, fa := range p.Deletes {
		if fa.Action == ActDownloadDelete {
			out[fa.Path] = FileEntry{}
		}
	}

	return out
}

// linkedBase is the base of path, reached through a link: REMOTE, so the local
// copy l wins every disagreement. It notes a server change that loses.
func (p *SyncPlan) linkedBase(path string, b, l, r FileEntry) FileEntry {
	if b.Hash != r.Hash && l.Hash != r.Hash {
		p.ReplacedRemote = append(p.ReplacedRemote, path)
	}

	return r
}

// add classifies path from its three sides and appends the action it needs.
func (p *SyncPlan) add(path string, b, l, r FileEntry) {
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
		return
	}

	p.Append(FileAction{
		Path:           path,
		Classification: cls,
		Action:         act,
		LocalSize:      l.Size,
		RemoteSize:     r.Size,
		LocalHash:      l.Hash,
		RemoteHash:     r.Hash,
		LocalExec:      l.Executable,
		RemoteExec:     r.Executable,
		BaseExec:       b.Executable,
	})
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
// unknown local or remote side is never drift.
func classifyExecutable(base, local, remote *bool) Classification {
	if base == nil {
		return classifyUnknownBase(local, remote)
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

// classifyUnknownBase settles a file last synced before the bit was tracked.
// Older CLIs dropped the bit and never added it, so where the two known sides
// disagree the executable one is right: upload it, or apply it locally.
func classifyUnknownBase(local, remote *bool) Classification {
	if local == nil || remote == nil || *local == *remote {
		return ClsUnchanged
	}

	if *local {
		return ClsLocalModified
	}

	return ClsRemoteModified
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
