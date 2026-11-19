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

package wizard

import (
	"fmt"
	"io"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// PortableManifest is a running workload's definition rendered so another
// install can deploy it: no workload binding, and every credential reference
// left for the reader to fill in. It is made before anything is written, so a
// workload id that does not resolve fails while nothing has changed yet.
type PortableManifest struct {
	// WorkloadID and Name say which workload the manifest was taken from.
	WorkloadID string
	Name       string
	// CredentialsPending is how many credential references now name the
	// placeholder.
	CredentialsPending int

	content []byte
}

// workloadPageSize is the server's ceiling for one page of the workload list.
const workloadPageSize = 100

// WorkloadsRunning lists the workloads whose artifact is exactly artifactID.
//
// The workload list cannot be filtered by artifact, so this reads every page
// and compares client-side. More than one match is possible because a locked
// artifact can back several workloads, and none is possible for an artifact
// that was never deployed, so the caller decides what each answer means.
func WorkloadsRunning(artifactID string) ([]workload.Workload, error) {
	var matches []workload.Workload

	for offset := 0; ; offset += workloadPageSize {
		page, err := listWorkloadsFn(workloadPageSize, offset, nil, "")
		if err != nil {
			return nil, fmt.Errorf("cannot list workloads to find the one that runs artifact %s: %w", artifactID, err)
		}

		for _, w := range page {
			if w.ArtifactID == artifactID {
				matches = append(matches, w)
			}
		}

		if len(page) < workloadPageSize {
			return matches, nil
		}
	}
}

// PreparePortable downloads a workload and renders its definition for another
// install.
//
// Nothing here looks at the project directory. Code is pulled by a later sync,
// so a provided Dockerfile is not on disk yet, and the deploy checks for it
// against the real tree when it runs.
//
// linkedRepositoryID is the lineage of the artifact the directory is linked
// to. A workload that runs another lineage is still rendered, because the
// caller named both and may know something this cannot, but the mismatch is
// said out loud on stderr.
func PreparePortable(workloadID, linkedRepositoryID string, stderr io.Writer) (PortableManifest, error) {
	live, err := fetchLive(workloadID)
	if err != nil {
		return PortableManifest{}, err
	}

	if stderr != nil && linkedRepositoryID != "" && live.RepositoryID != linkedRepositoryID {
		fmt.Fprintf(stderr,
			"Warning: workload %s does not run this artifact's lineage, so its definition may not describe the code you are linking.\n",
			workloadID)
	}

	portable, pending := live.Portable()

	applied, err := portable.Apply(portable.Defaults())
	if err != nil {
		return PortableManifest{}, err
	}

	content, err := applied.Render()
	if err != nil {
		return PortableManifest{}, err
	}

	// Parsed with no directory so the Dockerfile check, which needs the code
	// this file is written ahead of, is left to the deploy.
	if err := checkRendered(content, "", authorLive); err != nil {
		return PortableManifest{}, err
	}

	warnSecretLiterals(stderr, live, live.Name, "the manifest")

	return PortableManifest{
		WorkloadID:         workloadID,
		Name:               live.Name,
		CredentialsPending: pending,
		content:            content,
	}, nil
}

// Write puts the manifest in dir and returns its path. A manifest already
// there is the user's and is never replaced: the error wraps manifest.ErrExists.
func (p PortableManifest) Write(dir string) (string, error) {
	path := manifest.Path(dir)

	return path, manifest.Write(path, p.content)
}
