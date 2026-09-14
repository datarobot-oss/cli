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

// LockfileChecker reports whether uv.lock in dir is still current with
// respect to pyproject.toml. Injected via Deps so tests can fake the exec.
//
// A non-nil error means the question could not be answered — uv is not
// installed, or the check would not run — which the phase reads as
// "unknown" rather than as a stale lock. Not knowing is not the same as
// knowing it is wrong: a project whose lockfile is current synced with no
// uv on PATH before this check existed, and has to keep doing so.
type LockfileChecker func(dir string) (current bool, err error)

// phaseLockfile keeps uv.lock in step with pyproject.toml for a Python
// project, before the phase-2 walk so whatever it writes is collected,
// diffed and uploaded by the normal pipeline with no changes downstream.
//
// Two shapes, because the consequences differ:
//
//   - No uv.lock at all: generate one. A missing lockfile already fails
//     the image build loudly ("Found pyproject.toml but no uv.lock"), so
//     this is a convenience — when it cannot run, warning and continuing
//     still leaves the user with an error that names the problem.
//   - A uv.lock that no longer matches pyproject.toml: re-lock, and
//     refuse the sync if that cannot be done. Nothing downstream catches
//     this one (see refreshLockfile), so continuing would ship the wrong
//     image silently.
//
// lockfileHint doubles as the once-only guard so phase 2's ignore-file
// check doesn't stack a second warning.
func phaseLockfile(e *Engine) error {
	if !fileExistsIn(e.projectDir, pyprojectFile) {
		return nil
	}

	if fileExistsIn(e.projectDir, uvLockFile) {
		return refreshLockfile(e)
	}

	return generateLockfile(e)
}

// generateLockfile writes the uv.lock a project with a pyproject.toml is
// missing. It never fails the sync: the image build refuses a project
// with no lockfile on its own, so the worst case is the error the user
// would have got anyway, with a warning here naming the fix.
func generateLockfile(e *Engine) error {
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

// refreshLockfile re-locks a uv.lock that no longer describes
// pyproject.toml, and refuses the sync when it cannot.
//
// A stale lock is the one case in this phase that nothing downstream
// catches. The generated image build installs the lockfile as given
// (`uv sync --frozen`), so an edited pyproject.toml is simply ignored:
// sync succeeds, the build goes green, and the image is missing the
// dependency that was added. The user meets it at container start as an
// ImportError with nothing pointing back here. The build cannot make this
// check itself — its builder stage has only pyproject.toml and uv.lock
// copied in, so `uv lock --check` there fails on changes the image never
// consumes, including a `dynamic = ["dependencies"]` project that can
// never be made to pass. The CLI is the only component holding the whole
// project tree, which is what the check needs (RAPTOR-20217).
//
// Hence the asymmetry with generateLockfile: a lockfile this cannot put
// right is a wrong image nothing else will stop, so it stops the sync.
// The exception is not knowing at all — with no uv on PATH there is
// nothing to compare, and refusing would break every project whose
// lockfile is current and whose CI has no uv, which synced fine before
// this check existed.
func refreshLockfile(e *Engine) error {
	current, err := e.lockfileCheckFn(e.projectDir)
	if err != nil {
		if errors.Is(err, errUvNotFound) {
			e.lockfileHint = "uv is not installed, so uv.lock could not be checked against pyproject.toml. " +
				"If they have diverged the image is built from the old lock — install uv and run `uv lock`, then re-sync."
		} else {
			e.lockfileHint = fmt.Sprintf("Could not check uv.lock against pyproject.toml (%v). "+
				"If they have diverged the image is built from the old lock — run `uv lock` and re-sync.", err)
		}

		log.Warn(e.lockfileHint)

		return nil
	}

	if current {
		return nil
	}

	log.Debug("uv.lock no longer matches pyproject.toml; re-locking with `uv lock`")

	if err := e.lockfileFn(e.projectDir); err != nil {
		return fmt.Errorf("uv.lock is out of date with pyproject.toml and could not be regenerated: %w. "+
			"The image would be built from the old lock, so nothing was uploaded — "+
			"run `uv lock` in the project and re-sync", err)
	}

	// Same reason generateLockfile re-checks the filesystem rather than
	// trusting the exit code: `uv lock` in a workspace member succeeds
	// while writing the lockfile at the workspace root, leaving this
	// directory's copy exactly as stale as it was.
	if current, err := e.lockfileCheckFn(e.projectDir); err == nil && !current {
		return errors.New("uv lock reported success but uv.lock is still out of date with pyproject.toml " +
			"(uv workspaces write the lockfile at the workspace root). The image would be built from the old " +
			"lock, so nothing was uploaded — run `uv lock` in the project and re-sync")
	}

	e.lockfileGenerated = true

	log.Warn("uv.lock was out of date with pyproject.toml — regenerated it; commit it to your repo")

	return nil
}

// runUvLock is the production LockfileRunner: `uv lock` in dir with the
// user's own environment, so their uv config, private indexes, and
// credentials all apply — exactly as if they ran it by hand.
func runUvLock(dir string) error {
	out, err := runUv(dir, "uv lock", "lock")
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("uv lock failed: %s", tailOf(out, 400))
	}

	// uv missing, or timed out: both already say so in their own words.
	return err
}

// runUvLockCheck is the production LockfileChecker: `uv lock --check` in
// dir with the user's own environment, so their uv config, private
// indexes, and credentials all apply — exactly as if they ran it by hand.
//
// A non-zero exit means "not current" rather than "could not run". uv
// exits non-zero both for a lockfile that needs updating and for a
// pyproject.toml it cannot resolve, and both want the same answer here:
// re-lock, and refuse if that does not work either. Only a failure to
// execute uv at all is reported as an error, because that is the case
// where the sync has to continue rather than refuse.
//
// Cheap enough to run on every sync of a Python project: against a
// current lockfile it resolves from the lock itself, with no network and
// no measurable cost. It reaches the index only when something has
// actually changed, which is the run that was about to be wrong.
func runUvLockCheck(dir string) (bool, error) {
	if _, err := runUv(dir, "uv lock --check", "lock", "--check"); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}

		// uv missing, or timed out: the caller has to tell these apart
		// from "stale", because they mean the sync continues rather
		// than refuses.
		return false, err
	}

	return true, nil
}

// runUv executes `uv <args...>` in dir with the user's own environment,
// so their uv config, private indexes, and credentials all apply —
// exactly as if they ran it by hand. It returns uv's combined output so a
// caller can carry uv's own reason into its message, and errUvNotFound
// when uv is not on PATH at all.
//
// label names the command in the timeout message; a caller distinguishes
// a non-zero exit from a failure to run at all with errors.As on
// *exec.ExitError.
func runUv(dir, label string, args ...string) (string, error) {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return "", errUvNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), uvLockTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, uvPath, args...)
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
			return out.String(), fmt.Errorf("%s timed out after %s", label, uvLockTimeout)
		}

		return out.String(), err
	}

	return out.String(), nil
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
