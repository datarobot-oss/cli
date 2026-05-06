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

	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
)

// phase2Manifests builds the LOCAL manifest by walking + hashing the
// project, and either fetches REMOTE from FilesAPI (when drifted) or
// copies it from BASE (the solo-developer fast path).
func phase2Manifests(e *Engine) error {
	matcher, err := ignore.New(e.projectDir)
	if err != nil {
		return fmt.Errorf("load .wapiignore: %w", err)
	}

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

	if !e.drifted {
		// Nobody else changed the remote since our last sync; skip the
		// allFiles round-trip and reuse BASE.
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
			if container.CodeRef == nil || container.CodeRef.Datarobot == nil {
				continue
			}

			dr := container.CodeRef.Datarobot

			return codeRefRef{CatalogID: dr.CatalogID, CatalogVersionID: dr.CatalogVersionID}
		}
	}

	return codeRefRef{}
}
