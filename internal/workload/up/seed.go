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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
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

	// The project directory decides whether a pull is even possible, and it
	// needs no network to do it: a directory that already looks like a project
	// is deployed as it is, and one that holds unrelated files is refused. Only
	// an empty directory reaches for the live artifact, so the reads that
	// resolve its code run after these gates rather than before — a transient
	// files-API failure must not block a deploy that never needed seeding.
	if !wizard.Detect(loaded.ProjectDir).SuspectDir() {
		// A directory that already looks like a project is the normal case: the
		// user has the code, and the deploy reads it as it always has.
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

	return pullLiveCode(loaded, live, opts)
}

// pullLiveCode reads the live artifact's code catalog and pulls it into the
// (already-confirmed-empty) project directory. It is reached only once the
// local-directory gates have decided a pull is both possible and needed, so
// the network reads it does are the reads no other run has to pay for.
func pullLiveCode(loaded Loaded, live Live, opts Options) error {
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

	return pullCode(loaded.ProjectDir, codeRef, files, live, loaded, opts)
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

	set := make(map[string]struct{}, len(paths))

	for _, path := range paths {
		if err := fileops.SafeRelPath(path); err != nil {
			return fmt.Errorf("the workload's code names an unsafe path %q: %w", path, err)
		}

		set[path] = struct{}{}
	}

	// Two catalog keys that differ only by case (App.py vs app.py) map to one
	// file on macOS and Windows, so writing them in turn would silently drop
	// one and seed a tree that is missing code the build needs. Caught before
	// any file is written, mirroring the check the sync engine makes.
	if cs := fileops.DetectCaseCollisions(set); len(cs) > 0 {
		return fmt.Errorf(
			"the workload's code cannot be pulled onto this filesystem: %s",
			fileops.FormatCaseCollisions(cs))
	}

	for _, path := range paths {
		if err := downloadInto(client, codeRef, dir, path, files[path]); err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.Stderr, "  Pulled %d file(s) from %s's current version into %s\n",
		len(paths), name(loaded, live), dir)

	return nil
}

// downloadInto writes one file of the version into dir, creating parents, and
// verifies it against the catalog's size and hash as it streams.
//
// A half-written file is removed on any failure rather than left on disk. The
// pull writes in place (the directory was empty), so a partial survivor is not
// staged away from the deploy: on a retry a truncated root marker (a cut-off
// pyproject.toml) makes the directory look like a real project, seeding is
// skipped, and the deploy builds from the corrupt tree — and a partial that is
// not a marker refuses the next run with a message the user can only clear by
// deleting files by hand. Verifying size and hash catches a silent truncation
// the writer itself did not report. This mirrors the sync engine's downloadOne.
func downloadInto(
	client filesapi.Client,
	codeRef *workload.DatarobotCodeRef,
	dir, path string,
	meta filesapi.FileMeta,
) error {
	dst := filepath.Join(dir, filepath.FromSlash(path))

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("cannot create the directory for %s: %w", path, err)
	}

	file, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}

	hasher := sha256.New()

	_, n, downloadErr := client.DownloadFile(
		codeRef.CatalogID, codeRef.CatalogVersionID, path, io.MultiWriter(file, hasher))

	closeErr := file.Close()

	if failure := verifyDownload(path, meta, n, hasher, downloadErr, closeErr); failure != nil {
		_ = os.Remove(dst)

		return failure
	}

	return nil
}

// verifyDownload reports the first thing wrong with a file that was just
// written: the download or close itself, a byte count short of the catalog's,
// or a checksum that does not match. nil means the file on disk is the file
// the catalog holds. The caller removes the partial when this returns non-nil.
func verifyDownload(
	path string,
	meta filesapi.FileMeta,
	n int64,
	hasher hash.Hash,
	downloadErr, closeErr error,
) error {
	switch {
	case downloadErr != nil:
		return errors.Join(fmt.Errorf("cannot download %s", path), downloadErr)

	case closeErr != nil:
		return fmt.Errorf("cannot finish writing %s: %w", path, closeErr)

	case meta.Size > 0 && n != meta.Size:
		return fmt.Errorf("download of %s is %d bytes, expected %d", path, n, meta.Size)

	case meta.Hash != "" && hex.EncodeToString(hasher.Sum(nil)) != meta.Hash:
		return fmt.Errorf("download of %s does not match its expected checksum", path)

	default:
		return nil
	}
}
