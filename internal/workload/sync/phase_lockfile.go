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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload/ignore"
)

const (
	pyprojectFile = "pyproject.toml"
	uvLockFile    = "uv.lock"

	// uvLockTimeout bounds `uv lock`: dependency resolution hits the
	// network and can hang on unreachable indexes.
	uvLockTimeout = 2 * time.Minute
)

// errUvNotFound reports that the uv binary is not on PATH. The phase
// maps it to an install hint instead of a generation-failure hint.
var errUvNotFound = errors.New("uv is not installed")

// LockfileRunner generates uv.lock in dir (used as the subprocess
// working directory). Injected via Deps so tests can fake the exec.
type LockfileRunner func(dir string) error

// phaseLockfile lazily generates uv.lock when the project has a
// pyproject.toml but no lock file. The workload-api image build requires
// uv.lock (it runs `uv sync --frozen`), so generating it before the
// phase-2 walk lets the normal diff/upload pipeline pick it up with no
// changes downstream. This phase never fails the sync: when uv is
// missing or resolution fails it logs a WARN with the fix and lets the
// sync proceed (lockfileHint doubles as the once-only guard so phase 2's
// ignore-file check doesn't stack a second warning).
func phaseLockfile(e *Engine) error {
	if !fileExistsIn(e.projectDir, pyprojectFile) {
		return nil
	}

	if fileExistsIn(e.projectDir, uvLockFile) {
		return nil
	}

	log.Debug("pyproject.toml has no uv.lock; generating one with `uv lock`")

	if err := e.lockfileFn(e.projectDir); err != nil {
		if errors.Is(err, errUvNotFound) {
			e.lockfileHint = "pyproject.toml found without uv.lock and uv is not installed. " +
				"The image build will fail until you add one — install uv and run `uv lock`, then re-sync."
		} else {
			e.lockfileHint = fmt.Sprintf("Could not generate uv.lock (%v). "+
				"The image build will fail until you add one — run `uv lock` and re-sync.", err)
		}

		log.Warn(e.lockfileHint)

		return nil
	}

	// Don't trust the runner's exit code alone: in a uv workspace member
	// directory `uv lock` succeeds but writes the lockfile at the workspace
	// root, so projectDir gains no uv.lock and the upload would silently
	// ship without one (and every future sync would rerun generation).
	if !fileExistsIn(e.projectDir, uvLockFile) {
		e.lockfileHint = "uv lock completed but did not create uv.lock in the project directory " +
			"(uv workspaces write the lockfile at the workspace root). The image build requires " +
			"uv.lock next to pyproject.toml in the synced directory — add one and re-sync."

		log.Warn(e.lockfileHint)

		return nil
	}

	e.lockfileGenerated = true

	log.Warn("Generated uv.lock from pyproject.toml — commit it to your repo")

	return nil
}

// runUvLock is the production LockfileRunner: `uv lock` in dir with the
// user's own environment, so their uv config, private indexes, and
// credentials all apply — exactly as if they ran it by hand.
func runUvLock(dir string) error {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return errUvNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), uvLockTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, uvPath, "lock")
	cmd.Dir = dir
	// With Stdout/Stderr wired to a buffer, Run waits for the pipe-copy
	// goroutines, not just the process — a uv child (build backend, git)
	// surviving the timeout kill while holding the pipe would block Run
	// indefinitely with the sync lock held. WaitDelay bounds that wait
	// (same grace period as internal/plugin/exec.go).
	cmd.WaitDelay = 5 * time.Second

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	log.Debug("Running command: " + cmd.String())

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("uv lock timed out after %s", uvLockTimeout)
		}

		return fmt.Errorf("uv lock failed: %s", tailOf(out.String(), 400))
	}

	return nil
}

// warnIfLockfileIgnored surfaces a warning when the ignore file excludes
// uv.lock: that silently defeats both a user-committed lock and the
// lockfile phase's generated one — the image build requires it, so warn
// rather than upload a context that is doomed to fail. Called from
// phase2 (the first point where the ignore matcher exists).
//
// The message names the matcher's own source file. A project still on the
// legacy name would otherwise be sent to edit a file it does not have.
func warnIfLockfileIgnored(e *Engine, matcher *ignore.Matcher) {
	if e.lockfileHint != "" {
		return
	}

	if !fileExistsIn(e.projectDir, pyprojectFile) || !fileExistsIn(e.projectDir, uvLockFile) {
		return
	}

	if !matcher.Match(uvLockFile, false) {
		return
	}

	name := matcher.Source()
	if name == "" {
		// A matcher built from in-memory patterns has no file behind it.
		// Naming nothing would send the user to edit thin air.
		name = ignore.FileName
	}

	e.lockfileHint = fmt.Sprintf("uv.lock is excluded by %s, so it will not be uploaded. "+
		"The image build requires it, so remove the pattern from %s and re-sync.", name, name)

	log.Warn(e.lockfileHint)
}

func fileExistsIn(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))

	return err == nil && !info.IsDir()
}

// tailOf returns the last n bytes of s — uv prints the resolution
// error at the end of its output.
func tailOf(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}

	return "…" + s[len(s)-n:]
}
