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
	"github.com/datarobot/cli/internal/workload/wapi"
)

// phase1Gather loads on-disk state, fetches the artifact, and computes
// the drift flag that decides whether Phase 2 calls allFiles or fast-paths.
func phase1Gather(e *Engine) error {
	cfg, err := wapi.LoadConfig(e.projectDir)
	if err != nil {
		return fmt.Errorf("read .wapi/config.json: %w", err)
	}

	e.config = cfg

	manifest, err := wapi.LoadManifest(e.projectDir)
	if err != nil {
		if errors.Is(err, wapi.ErrNotInitialized) {
			return fmt.Errorf("manifest missing: %w", err)
		}

		return fmt.Errorf("read .wapi/manifest.json: %w", err)
	}

	e.base = baseFromManifest(manifest)

	art, err := e.getArtifactFn(cfg.ArtifactID)
	if err != nil {
		return fmt.Errorf("fetch artifact %s: %w", cfg.ArtifactID, err)
	}

	if art.IsLocked() {
		return errors.New("artifact is locked (immutable); cannot sync. Create a new draft artifact in the UI to continue")
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
