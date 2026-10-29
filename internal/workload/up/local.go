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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/docker"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/tui"
)

// A local build makes the image here, with the user's Docker, and pushes it to
// the address the platform issued for the artifact. Everything around it is
// the build path's: the artifact exists before the image, a new version for a
// change, a leftover picked up after an interrupted run. What it replaces is
// the code sync and the platform build, so no code is uploaded.
var (
	dockerAvailableFn = docker.Available
	dockerBuildFn     = docker.Build
	dockerPushFn      = docker.Push
	treeHashFn        = treeHash
	apiTokenFn        = func() string { return viperx.GetString(config.DataRobotAPIKey) }
)

// dockerReady refuses a run that will build with Docker when Docker does not
// answer, before anything is created or started.
func dockerReady(ctx context.Context, plan Plan) error {
	if !plan.Code.Local || !buildsImage(plan) {
		return nil
	}

	return dockerAvailableFn(ctx)
}

// localCodeChange measures the working tree against the last push from this
// checkout. There is no code store to diff against, so the answer is a hash
// of the tree the sync engine would upload, and a changed one is one change.
func localCodeChange(loaded Loaded, live Live) (CodeChange, error) {
	hash, err := treeHashFn(loaded.ProjectDir)
	if err != nil {
		return CodeChange{}, fmt.Errorf("cannot inspect the working tree: %w", err)
	}

	change := CodeChange{Applies: true, Local: true, TreeHash: hash}

	if !projectLinkedFn(loaded.ProjectDir) {
		change.FirstDeploy = true

		return change, nil
	}

	cfg, err := loadProjectFn(loaded.ProjectDir)
	if err != nil {
		return CodeChange{}, fmt.Errorf("cannot read what this project last pushed: %w", err)
	}

	change.TreeChanged = deref(cfg.LastPushedTreeHash) != hash

	// The unserved rule for pushed images: this checkout pushed to the version
	// it is linked to and marked pending, and the workload is not running it.
	// The pending mark is what keeps a version someone else rolled out since
	// from being rolled back.
	change.Unserved = live.ArtifactID != "" &&
		cfg.ArtifactID != live.ArtifactID &&
		deref(cfg.PendingRolloutArtifactID) == cfg.ArtifactID &&
		deref(cfg.LastPushedArtifactID) == cfg.ArtifactID

	return change, nil
}

// treeHash is one hash over every file the sync engine would upload, under
// the same ignore rules, with each file's path, content and executable bit.
func treeHash(dir string) (string, error) {
	matcher, err := ignore.New(dir)
	if err != nil {
		return "", err
	}

	entries, err := fileops.Walk(dir, matcher.Match, nil)
	if err != nil {
		return "", err
	}

	h := sha256.New()

	for _, entry := range entries {
		sum, _, mode, err := fileops.HashFileMode(entry.AbsPath)
		if err != nil {
			return "", err
		}

		executable := fileops.LocalExecutable(mode)

		fmt.Fprintf(h, "%s\x00%s\x00%t\n", entry.RelPath, sum, executable != nil && *executable)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// pushLocally builds the project with Docker and pushes it to made's address,
// returning the address. The same tree going to the same artifact again is
// skipped: the image is already there.
func pushLocally(ctx context.Context, projectDir string, made version, code CodeChange, opts Options,
	report *reporter,
) (string, error) {
	ref, err := pushTarget(made)
	if err != nil {
		return "", err
	}

	if !opts.ForceBuild && pushedAlready(projectDir, made.ID, code.TreeHash) {
		report.say("  Image is up to date; no rebuild needed.\n")

		return ref.Address(), nil
	}

	err = report.stream(ctx, "Building the image with Docker", func(ctx context.Context, say func(string, lipgloss.Style)) error {
		return dockerBuildFn(ctx, projectDir, ref.Address(), func(line string) { say(line, tui.HintStyle) })
	})
	if err != nil {
		return "", err
	}

	var digest string

	err = report.stream(ctx, "Pushing the image", func(ctx context.Context, say func(string, lipgloss.Style)) error {
		pushed, pushErr := dockerPushFn(ctx, ref, apiTokenFn(), func(line string) { say(line, tui.HintStyle) })
		digest = pushed

		return pushErr
	})
	if err != nil {
		return "", err
	}

	report.say("  Pushed %s@%s\n", ref.Address(), digest)
	recordPush(projectDir, made.ID, code.TreeHash, digest)

	return ref.Address(), nil
}

// pushTarget is the address the platform issued for made: a container that
// names neither an image nor a build is given one at create.
func pushTarget(made version) (docker.Reference, error) {
	uri := made.ImageURI

	if uri == "" {
		doc, err := getArtifactDocFn(made.ID)
		if err != nil {
			return docker.Reference{}, fmt.Errorf("cannot read where to push artifact %s's image: %w", made.ID, err)
		}

		uri, _ = workload.PrimaryContainerInDocument(doc)[keyImageURI].(string)
	}

	ref, ok := docker.ParseReference(uri)

	switch {
	case !ok:
		return docker.Reference{}, fmt.Errorf(
			"artifact %s has no address to push its image to (its image is %q). The platform issues one "+
				"only where pushing images to DataRobot is enabled", made.ID, uri)
	case ref.Digest != "" || ref.ArtifactID != made.ID:
		return docker.Reference{}, fmt.Errorf(
			"artifact %s runs the image %s, which a push to it would not change", made.ID, uri)
	}

	return ref, nil
}

// pushedAlready reports that this tree was last pushed to this artifact.
func pushedAlready(projectDir, artifactID, treeHash string) bool {
	if treeHash == "" || !projectLinkedFn(projectDir) {
		return false
	}

	cfg, err := loadProjectFn(projectDir)

	return err == nil && deref(cfg.LastPushedArtifactID) == artifactID && deref(cfg.LastPushedTreeHash) == treeHash
}

// recordPush notes what was pushed where. Best effort: not writing it costs a
// rebuild, which Docker's cache makes cheap.
func recordPush(projectDir, artifactID, treeHash, digest string) {
	if !projectLinkedFn(projectDir) {
		return
	}

	cfg, err := loadProjectFn(projectDir)
	if err != nil {
		return
	}

	cfg.LastPushedArtifactID, cfg.LastPushedDigest = &artifactID, &digest

	if treeHash != "" {
		cfg.LastPushedTreeHash = &treeHash
	}

	if err := saveProjectFn(projectDir, cfg); err != nil {
		log.Debug("cannot record the pushed image", "project_dir", projectDir, "err", err)
	}
}

// pinnedImage is the image a locked version of a pushed image runs, read back
// after the lock pinned it; the address it was pushed to when that read fails.
func pinnedImage(artifactID, address string) string {
	doc, err := getArtifactDocFn(artifactID)
	if err != nil {
		return address
	}

	if uri, _ := workload.PrimaryContainerInDocument(doc)[keyImageURI].(string); uri != "" {
		return uri
	}

	return address
}
