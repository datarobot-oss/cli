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

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
)

// hashEntriesFn is a test seam in the style of availableBytesFn
// (diskspace.go): tests swap it to inject a local manifest that a real
// filesystem walk can never produce. Case-insensitive hosts (macOS, Windows)
// collapse case-colliding paths, so the collision check below can only be
// exercised on every host by injecting the manifest rather than walking one.
var hashEntriesFn = hashEntries

// warnSkippedSymlinks sorts the skipped symlinks by path, so notices and the
// structured field are deterministic, and logs them from the phase like the
// shadow warning: the user hears it even when a later phase fails before
// anything renders. The prose stops at SymlinkNoticeBound; the engine field
// carries every symlink regardless.
func warnSkippedSymlinks(e *Engine) {
	sort.Slice(e.skippedSymlinks, func(i, j int) bool {
		return e.skippedSymlinks[i].Path < e.skippedSymlinks[j].Path
	})

	if e.opts.Quiet {
		return
	}

	for i, s := range e.skippedSymlinks {
		if i >= SymlinkNoticeBound {
			break
		}

		log.Warn(skippedSymlinkNotice(s))
	}

	if len(e.skippedSymlinks) > SymlinkNoticeBound {
		log.Warn(fmt.Sprintf(
			"skipped symlink: and %d more symlink(s) were not uploaded or synced (see the plan JSON for the full list)",
			len(e.skippedSymlinks)-SymlinkNoticeBound))
	}
}

// phase2Manifests builds the LOCAL manifest by walking + hashing the
// project, and either fetches REMOTE from FilesAPI (when drifted) or
// copies it from BASE (the solo-developer fast path).
func phase2Manifests(e *Engine) error {
	matcher, err := ignore.New(e.projectDir)
	if err != nil {
		// No filename here: the matcher reads two candidates and the wrapped
		// error already names the one that failed.
		return fmt.Errorf("load ignore patterns: %w", err)
	}

	e.ignoreNotice = matcher.Notice()

	// Logged rather than returned: a second ignore file means patterns the user
	// wrote are inert, which they need to hear even if a later phase fails
	// before anything gets a chance to render a returned notice.
	if shadow := matcher.ShadowWarning(); shadow != "" {
		log.Warn(shadow)
	}

	warnIfLockfileIgnored(e, matcher)

	// The walk's symlink arm returns before the ignore check (walk.go tests
	// ModeSymlink before calling ignore), so filtering must happen here rather
	// than in the walker. Without it, a symlink the user deliberately .drignore'd
	// or that is system-excluded (e.g. named .git) would still be announced — the
	// classic unfiltered-warning trap. matcher.Match applies both the user's
	// .drignore patterns and the hardcoded system excludes, with the same
	// case-folding rules used for regular files.
	walkOnSymlink := func(rel, _ string, isDir, dangling bool) {
		matched := matcher.Match(rel, isDir)

		// A dangling link's kind is unknowable, so a directory-only pattern
		// ("node_modules/") cannot match through the isDir=false branch.
		// Treat it as excluded when either spelling matches so a dangling
		// node_modules link is still filtered rather than warned about.
		if dangling {
			matched = matcher.Match(rel, false) || matcher.Match(rel, true)
		}

		if matched {
			return
		}

		e.skippedSymlinks = append(e.skippedSymlinks, SkippedSymlink{Path: rel, IsDir: isDir})
	}

	entries, err := fileops.Walk(e.projectDir, matcher.Match, walkOnSymlink)
	if err != nil {
		return fmt.Errorf("walk project directory: %w", err)
	}

	warnSkippedSymlinks(e)

	local, err := hashEntriesFn(entries)
	if err != nil {
		return err
	}

	e.local = local

	if cs := caseCollisionsFromManifest(local); len(cs) > 0 {
		return fmt.Errorf("%s", fileops.FormatCaseCollisions(cs))
	}

	return resolveRemote(e)
}

// trustsBase reports the solo-developer fast path: nobody else changed the
// remote since the last sync, so BASE stands in for it. --verify opts out,
// since its point is to check BASE against the server, on dry-run too. So
// does a base that lacks the executable bits, which lists the remote once to
// learn them.
func trustsBase(e *Engine, codeRef codeRefRef) bool {
	if e.drifted || e.opts.Verify {
		return false
	}

	if needsExecutableBackfill(e, codeRef) {
		e.execBackfill = true

		return false
	}

	return true
}

// needsExecutableBackfill reports a fast-path sync that should list the
// remote anyway: a base written before the executable bit was tracked cannot
// say what the catalog holds, unless the server is known not to report it.
func needsExecutableBackfill(e *Engine, codeRef codeRefRef) bool {
	return hasUnknownExecutable(e.base) && !e.config.ExecutableUnreported &&
		codeRef.CatalogID != "" && e.remoteVer != ""
}

// resolveRemote fills e.remote: copied from BASE on the fast path, empty on a
// first sync, or listed from the Files API. Split out of the phase body, which
// is at the complexity ceiling.
func resolveRemote(e *Engine) error {
	codeRef := codeRefOrEmpty(e)

	if trustsBase(e, codeRef) {
		e.remote = copyManifest(e.base)

		return nil
	}

	if codeRef.CatalogID == "" || e.remoteVer == "" {
		// First sync against an empty artifact: remote manifest is empty.
		e.remote = RemoteManifest{}

		return nil
	}

	remote, err := e.files.AllFiles(codeRef.CatalogID, e.remoteVer)
	if err != nil && e.execBackfill {
		// The backfill only learns the bits; without it the sync is the one it was.
		log.Debug("Could not list the remote to learn executable bits", "error", err)

		e.execBackfill = false
		e.remote = copyManifest(e.base)

		return nil
	}

	if err != nil {
		return fmt.Errorf("fetch remote manifest: %w", err)
	}

	e.remote = FromFilesAPI(remote)
	e.remoteListed = true

	// Only a verify-forced fetch on a non-drifted artifact checks BASE's
	// claim: here — and only here — BASE claims to describe exactly the
	// version just fetched, so a mismatch is a lie worth reporting. On a
	// drifted artifact the remote is a newer version by design, and
	// BASE-vs-REMOTE differences are ordinary drift, not findings.
	maybeDetectDivergence(e)

	return nil
}

// hashEntries hashes each entry sequentially. Concurrency would help
// only marginally for typical projects since Phase 5 network is the
// real bottleneck.
func hashEntries(entries []fileops.Entry) (LocalManifest, error) {
	out := make(LocalManifest, len(entries))

	for _, ent := range entries {
		hash, size, mode, err := fileops.HashFileMode(ent.AbsPath)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", ent.RelPath, err)
		}

		out[ent.RelPath] = FileEntry{Hash: hash, Size: size, Executable: fileops.LocalExecutable(mode)}
	}

	return out, nil
}

func caseCollisionsFromManifest(m LocalManifest) []fileops.CaseCollision {
	set := make(map[string]struct{}, len(m))
	for k := range m {
		set[k] = struct{}{}
	}

	return fileops.DetectCaseCollisions(set)
}

func copyManifest(in BaseManifest) BaseManifest {
	out := make(BaseManifest, len(in))
	for k, v := range in {
		out[k] = v
	}

	return out
}

type codeRefRef struct {
	CatalogID        string
	CatalogVersionID string
}

func codeRefOrEmpty(e *Engine) codeRefRef {
	if e.artifact == nil {
		return codeRefRef{}
	}

	if e.config.CatalogID != nil && *e.config.CatalogID != "" {
		// Local config's catalog ID is pinned for the DRAFT lifetime;
		// the artifact's codeRef may have been bumped by another writer.
		return codeRefRef{CatalogID: *e.config.CatalogID, CatalogVersionID: e.remoteVer}
	}

	if cr := refFromArtifact(e); cr.CatalogID != "" {
		return cr
	}

	return codeRefRef{}
}

func refFromArtifact(e *Engine) codeRefRef {
	if e.artifact == nil {
		return codeRefRef{}
	}

	for _, group := range e.artifact.Spec.ContainerGroups {
		for _, container := range group.Containers {
			if container.ImageBuildConfig == nil ||
				container.ImageBuildConfig.CodeRef == nil ||
				container.ImageBuildConfig.CodeRef.Datarobot == nil {
				continue
			}

			dr := container.ImageBuildConfig.CodeRef.Datarobot

			return codeRefRef{CatalogID: dr.CatalogID, CatalogVersionID: dr.CatalogVersionID}
		}
	}

	return codeRefRef{}
}
