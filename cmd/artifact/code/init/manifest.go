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

package initcmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/spf13/cobra"
)

var findWorkloadsFn = workload.FindWorkloadsByArtifact

type portableManifest struct {
	workloadID string
	name       string
	content    []byte
	pending    int
}

// getManifest builds the manifest for --with-manifest, or returns nil when none is wanted.
func getManifest(cmd *cobra.Command, art *workload.Artifact) (*portableManifest, error) {
	workloadID, err := manifestWorkload(cmd, art)
	if err != nil || workloadID == "" {
		return nil, err
	}

	return buildManifest(cmd.ErrOrStderr(), workloadID, art)
}

// manifestWorkload is the workload to take the manifest from, "" when none is wanted or found.
func manifestWorkload(cmd *cobra.Command, art *workload.Artifact) (string, error) {
	withManifest, _ := cmd.Flags().GetBool("with-manifest")
	workloadID, _ := cmd.Flags().GetString("workload-id")

	switch {
	case workloadID != "" && !withManifest:
		return "", errors.New("--workload-id only applies with --with-manifest")
	case !withManifest || workloadID != "":
		return workloadID, nil
	}

	return findWorkload(cmd.ErrOrStderr(), art)
}

func buildManifest(stderr io.Writer, workloadID string, art *workload.Artifact) (*portableManifest, error) {
	live, err := wizard.FetchLive(workloadID)
	if err != nil {
		return nil, err
	}

	if art.ArtifactRepositoryID != "" && live.RepositoryID != art.ArtifactRepositoryID {
		fmt.Fprintf(stderr, "Warning: workload %s runs a different artifact lineage than %s.\n", workloadID, art.ID)
	}

	portable, pending := live.Portable()

	applied, err := portable.Apply(portable.Defaults())
	if err != nil {
		return nil, err
	}

	content, err := applied.Render()
	if err != nil {
		return nil, err
	}

	if err := wizard.CheckLive(content); err != nil {
		return nil, err
	}

	wizard.WarnSecretLiterals(stderr, live, live.Name, "the manifest")

	return &portableManifest{workloadID: workloadID, name: live.Name, content: content, pending: pending}, nil
}

// findWorkload returns the one workload that runs the artifact, "" when none does.
func findWorkload(stderr io.Writer, art *workload.Artifact) (string, error) {
	found, err := findWorkloadsFn(art.ID)
	if err != nil {
		return "", err
	}

	switch len(found) {
	case 0:
		fmt.Fprintf(stderr, "No workload runs artifact %s, so no %s was written. "+
			"To use a workload that runs another version, pass --workload-id.\n", art.ID, manifest.FileName)

		return "", nil
	case 1:
		return found[0].ID, nil
	}

	lines := make([]string, 0, len(found))
	for _, w := range found {
		lines = append(lines, fmt.Sprintf("  %s  %s (%s)", w.ID, w.Name, w.Status))
	}

	return "", fmt.Errorf("%d workloads run artifact %s; pass --workload-id to choose one:\n%s",
		len(found), art.ID, strings.Join(lines, "\n"))
}

// write puts the manifest in dir. An existing manifest is left alone.
func (m *portableManifest) write(stderr io.Writer, dir string, result *initResult) error {
	path := manifest.Path(dir)

	err := manifest.Write(path, m.content)
	if errors.Is(err, manifest.ErrExists) {
		fmt.Fprintf(stderr, "%s already exists and was left as it is.\n", wizard.ShortPath(path))

		return nil
	}

	if err != nil {
		return fmt.Errorf("linked the artifact, but could not write the manifest: %w", err)
	}

	result.ManifestPath = path
	result.ManifestCredentialsPending = &m.pending

	fmt.Fprintf(stderr, "✓ Wrote %s from workload %s (%s), with no workload binding.\n",
		wizard.ShortPath(path), m.name, m.workloadID)

	if m.pending > 0 {
		fmt.Fprintf(stderr, "  %d credential %s name %s: replace each with a credential id from the target install.\n",
			m.pending, wizard.Plural(m.pending, "reference", "references"), manifest.CredentialPlaceholder)
	}

	return nil
}
