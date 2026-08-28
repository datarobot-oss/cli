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
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/workload"
	wlmanifest "github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// phase1Gather loads on-disk state, fetches the artifact, and computes
// the drift flag that decides whether Phase 2 calls allFiles or fast-paths.
func phase1Gather(e *Engine) error {
	cfg, err := wapi.LoadConfig(e.projectDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", wapi.ConfigPath(e.projectDir), err)
	}

	e.config = cfg

	manifest, err := wapi.LoadManifest(e.projectDir)
	if err != nil {
		if errors.Is(err, wapi.ErrNotInitialized) {
			return fmt.Errorf("manifest missing: %w", err)
		}

		return fmt.Errorf("read manifest.json in %s: %w", wapi.Dir(e.projectDir), err)
	}

	e.base = baseFromManifest(manifest)

	art, err := e.artifacts.Get(cfg.ArtifactID)
	if err != nil {
		return fmt.Errorf("fetch artifact %s: %w", cfg.ArtifactID, err)
	}

	// A locked artifact is immutable, so nothing can be pushed into it. That
	// refuses a sync, but it must not refuse a plan: `dr workload up` asks for
	// this diff before it decides what to do with the code, and on a locked
	// version what it decides is to mint a new one and roll onto it. Refusing
	// to count the files refuses the very deploy that gets past the lock, which
	// is how a locked artifact became a one-way door. Phase 5 checks again, so
	// the exemption cannot let a write through.
	if art.IsLocked() {
		if !e.previewOnly() {
			return fmt.Errorf(
				"artifact %s is locked (immutable); cannot sync.\n"+
					"  A project with a %s deploys past this on its own: 'dr workload up' creates a new "+
					"version, points this directory at it, and rolls onto it.\n"+
					"  Otherwise make one with 'dr artifact create' (or in the DataRobot UI), then point this "+
					"directory at it with 'dr artifact code init --force <new-id>'",
				art.ID, wlmanifest.FileName)
		}

		e.lockedNote = fmt.Sprintf(
			"Artifact %s is locked, so this is a preview only: syncing into it needs a new version.", art.ID)
	}

	e.artifact = art

	if codeRef := workload.ExtractCodeRef(*art); codeRef != nil {
		e.remoteVer = codeRef.CatalogVersionID
	}

	e.drifted = e.remoteVer != "" && e.remoteVer != ptrOrEmpty(cfg.LastSyncedVersionID)

	return nil
}

func baseFromManifest(m wapi.Manifest) BaseManifest {
	out := make(BaseManifest, len(m.Files))
	for k, v := range m.Files {
		out[k] = FileEntry{Hash: v.Hash, Size: v.Size}
	}

	return out
}
