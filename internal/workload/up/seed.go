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

package up

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
)

// filesClientFn builds the client that pulls an existing workload's code. A
// seam so tests can serve files without a network.
var filesClientFn = filesapi.New

// seedIfMinting pulls the workload's code when this run is one that pushes the
// working tree. Only a version-minting run (a roll, or a create from source)
// reads the tree; a retune, a start, or an unchanged run leaves it alone, and
// none of them should read the live artifact or touch the directory.
func seedIfMinting(loaded Loaded, live Live, plan Plan, opts Options) error {
	if !plan.MintsVersion() {
		return nil
	}

	return seedFromLiveArtifact(loaded, live, opts)
}

// seedFromLiveArtifact pulls a bound workload's current code into an empty
// project directory before the deploy reads the working tree.
//
// Binding to an existing source-built workload has no download step: `config`
// writes the live spec into the manifest, but never the code the artifact was
// built from. So an `up` run from a directory that does not already hold that
// code would roll a new version from whatever is there — an empty tree becomes
// an empty artifact that fails the build with "cannot detect project runtime",
// minutes later and after the workload has already been touched. This closes
// that gap: when the directory is empty, the code is pulled down so the deploy
// builds from the real thing; when it holds unrelated files, the run is
// refused rather than allowed to replace the workload's code with them.
//
// It runs only for the one shape that needs it: a source-built workload, bound
// and already existing, in a directory this CLI has not linked yet. Every
// other run is left untouched.
func seedFromLiveArtifact(loaded Loaded, live Live, opts Options) error {
	if !seedApplies(loaded, live) {
		return nil
	}

	codeRef, err := artifactCodeRef(live.ArtifactID)
	if err != nil || codeRef == nil {
		// A nil ref (no code catalog: a published image) is nothing to pull.
		return err
	}

	files, err := filesClientFn().AllFiles(codeRef.CatalogID, codeRef.CatalogVersionID)
	if err != nil {
		return fmt.Errorf("cannot read the code of workload %s to pull it: %w", live.WorkloadID, err)
	}

	if !hasRealCode(files) {
		// The workload's own artifact is empty too (only the ignore file, or
		// nothing). There is nothing to seed, and the build guard elsewhere is
		// what speaks to a runtime that cannot be detected.
		return nil
	}

	return seedLocalDir(loaded, live, codeRef, files, opts)
}

// seedApplies reports whether this run is the one shape a pull is for: a
// source-built workload, bound and already existing, in a directory the CLI
// has not linked yet. A linked project is past first-bind and its incremental
// sync knows its own base; a published image has no code to pull.
func seedApplies(loaded Loaded, live Live) bool {
	mode := loaded.Manifest.BuildMode()
	if mode != manifest.BuildModeDockerfile && mode != manifest.BuildModeGenerated {
		return false
	}

	return live.ArtifactID != "" && !projectLinkedFn(loaded.ProjectDir)
}

// seedLocalDir decides what to do about the project directory: leave a real
// project as it is, pull into an empty one, or refuse one that holds files
// that are not a recognised project and are not this workload's code.
func seedLocalDir(
	loaded Loaded,
	live Live,
	codeRef *workload.DatarobotCodeRef,
	files map[string]filesapi.FileMeta,
	opts Options,
) error {
	// A directory that already looks like a project is the normal case: the
	// user has the code, and the deploy reads it as it always has.
	if !wizard.Detect(loaded.ProjectDir).SuspectDir() {
		return nil
	}

	empty, err := looksEmpty(loaded.ProjectDir)
	if err != nil {
		return err
	}

	if !empty {
		// Files, but not a recognised project, and not linked to this
		// workload. Rolling them would replace the workload's code with
		// whatever this is; deploying its own code needs an empty directory to
		// pull into, or the project root.
		return fmt.Errorf(
			"refusing to deploy: %s has files but no recognised project layout, and workload %s already has "+
				"code this run would replace. Deploy from the project root, or start from an empty directory "+
				"to pull the workload's current code first",
			loaded.ProjectDir, name(loaded, live))
	}

	return pullCode(loaded.ProjectDir, codeRef, files, live, loaded, opts)
}

// artifactCodeRef reads the code catalog behind the artifact the workload is
// running, nil when the artifact is built from a published image rather than
// from source.
func artifactCodeRef(artifactID string) (*workload.DatarobotCodeRef, error) {
	artifact, err := getArtifactFn(artifactID)
	if err != nil {
		return nil, fmt.Errorf("cannot read artifact %s to pull its code: %w", artifactID, err)
	}

	return workload.ExtractCodeRef(*artifact), nil
}

// hasRealCode reports whether the file set is more than the CLI's own ignore
// file. An artifact carrying only .drignore has no code worth pulling.
func hasRealCode(files map[string]filesapi.FileMeta) bool {
	for path := range files {
		if path != ignore.FileName {
			return true
		}
	}

	return false
}

// looksEmpty reports whether the directory holds nothing but the files the CLI
// writes itself: the state directory, the manifest, and the ignore file. Those
// are the residue of an earlier bind or a failed run, not the user's code, so a
// directory holding only them is still a directory to pull into.
func looksEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", dir, err)
	}

	for _, entry := range entries {
		switch entry.Name() {
		case wapi.RootDirName, manifest.FileName, ignore.FileName, ".git":
			continue
		default:
			return false, nil
		}
	}

	return true, nil
}

// pullCode downloads every file of the workload's current version into the
// project directory. The directory is empty (looksEmpty gated the call), so
// the files are written in place rather than staged and swapped.
func pullCode(
	dir string,
	codeRef *workload.DatarobotCodeRef,
	files map[string]filesapi.FileMeta,
	live Live,
	loaded Loaded,
	opts Options,
) error {
	client := filesClientFn()

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	for _, path := range paths {
		if err := fileops.SafeRelPath(path); err != nil {
			return fmt.Errorf("the workload's code names an unsafe path %q: %w", path, err)
		}
	}

	for _, path := range paths {
		if err := downloadInto(client, codeRef, dir, path); err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.Stderr, "  Pulled %d file(s) from %s's current version into %s\n",
		len(paths), name(loaded, live), dir)

	return nil
}

// downloadInto writes one file of the version into dir, creating parents.
func downloadInto(client filesapi.Client, codeRef *workload.DatarobotCodeRef, dir, path string) error {
	dst := filepath.Join(dir, filepath.FromSlash(path))

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("cannot create the directory for %s: %w", path, err)
	}

	file, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}

	defer func() { _ = file.Close() }()

	if _, _, err := client.DownloadFile(codeRef.CatalogID, codeRef.CatalogVersionID, path, file); err != nil {
		return errors.Join(fmt.Errorf("cannot download %s", path), err)
	}

	return nil
}
