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

	local, err := hashEntries(entries)
	if err != nil {
		return err
	}

	e.local = local

	if cs := caseCollisionsFromManifest(local); len(cs) > 0 {
		return fmt.Errorf("%s", fileops.FormatCaseCollisions(cs))
	}

	return loadRemote(e)
}

// loadRemote decides where REMOTE comes from: copied from BASE by the
// solo-developer fast path, empty on a first sync, or fetched from the
// FilesAPI. It is split out of the phase body because the fast-path guard
// and the divergence check each carry compound conditions, and the phase
// function is at the complexity ceiling.
func loadRemote(e *Engine) error {
	if !e.drifted && !e.opts.Verify {
		// Nobody else changed the remote since our last sync; skip the
		// allFiles round-trip and reuse BASE. --verify opts out of this
		// trust: its whole point is to check that BASE still describes the
		// server, which requires actually asking the server. The bypass is
		// unconditional on dry-run — a non-dry-run verify run must fetch too.
		e.remote = copyManifest(e.base)

		return nil
	}

	codeRef := codeRefOrEmpty(e)
	if codeRef.CatalogID == "" || e.remoteVer == "" {
		// First sync against an empty artifact: remote manifest is empty.
		e.remote = RemoteManifest{}

		return nil
	}

	remote, err := e.files.AllFiles(codeRef.CatalogID, e.remoteVer)
	if err != nil {
		return fmt.Errorf("fetch remote manifest: %w", err)
	}

	e.remote = FromFilesAPI(remote)

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
		hash, size, err := fileops.HashFile(ent.AbsPath)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", ent.RelPath, err)
		}

		out[ent.RelPath] = FileEntry{Hash: hash, Size: size}
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
