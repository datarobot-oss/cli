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

	var skippedSymlinks []string

	walkOnSymlink := func(rel, _ string) {
		skippedSymlinks = append(skippedSymlinks, rel)
	}

	entries, err := fileops.Walk(e.projectDir, matcher.Match, walkOnSymlink)
	if err != nil {
		return fmt.Errorf("walk project directory: %w", err)
	}

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

// resolveRemote fills e.remote: listed from the Files API when the remote
// moved or the base lacks the executable bits, copied from BASE otherwise.
func resolveRemote(e *Engine) error {
	codeRef := codeRefOrEmpty(e)

	if !e.drifted {
		// A base written before the executable bit was tracked cannot say what
		// the catalog holds, so the remote is listed once to learn it.
		if !hasUnknownExecutable(e.base) || codeRef.CatalogID == "" || e.remoteVer == "" {
			// Nobody else changed the remote since our last sync; skip the
			// allFiles round-trip and reuse BASE.
			e.remote = copyManifest(e.base)

			return nil
		}

		e.execBackfill = true
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
