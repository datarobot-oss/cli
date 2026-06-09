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

package workload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/workload/wapi"
)

const (
	ArtifactIDSourceExplicit = "explicit"
	ArtifactIDSourceWAPI     = "wapi"
)

// ResolveArtifactID returns the artifact id to operate on. When explicit is
// non-empty it is returned verbatim with source "explicit". Otherwise the
// current working directory is searched for a .wapi/config.json (the same
// project-linked sync state managed by 'dr workload code init/sync') and the
// id is read from there with source "wapi".
//
// Returns a user-facing error when neither source provides one so callers can
// surface the .wapi-init hint without further wrapping.
func ResolveArtifactID(explicit string) (string, string, error) {
	if explicit != "" {
		return explicit, ArtifactIDSourceExplicit, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("resolve current directory: %w", err)
	}

	absDir, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("resolve %s: %w", cwd, err)
	}

	cfg, err := wapi.LoadConfig(absDir)
	if err != nil {
		if errors.Is(err, wapi.ErrNotInitialized) {
			return "", "", fmt.Errorf("artifact id not provided and no .wapi project in %s. Run 'dr workload code init <artifact-id>' or pass the id explicitly", absDir)
		}

		return "", "", fmt.Errorf("read .wapi/config.json: %w", err)
	}

	return cfg.ArtifactID, ArtifactIDSourceWAPI, nil
}
