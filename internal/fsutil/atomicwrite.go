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

package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultFileMode is the permission CLI-written data files get: readable by
// anyone who can read the project, writable by the owner. Exported so callers
// that open a file themselves (an append-only log, say) stay consistent with
// what AtomicWriteFile leaves behind.
const DefaultFileMode os.FileMode = 0o644

// ownerOnlyFileMode is what a new file gets when the umask cannot be measured.
// Narrower than DefaultFileMode on purpose: the measurement exists to stop a
// file being widened past what the user's umask allows, so failing it must not
// hand out the very mode that was in question.
const ownerOnlyFileMode os.FileMode = 0o600

// targetMode is the permission the written file should end up with.
//
// Replacing a file keeps the mode it already had: a user who tightened
// something to 0600 should not find it widened by an unrelated edit. A new
// file gets what a plain create would give it, umask included, because
// os.CreateTemp makes the temp file 0600 and chmodding to a fixed mode would
// hand a developer with a restrictive umask a wider file than everything else
// they write.
//
// probePath is where the umask gets measured: a scratch path in the directory
// the file will live in, never the destination itself. Measuring on the
// destination would create it here, and any later failure would then leave an
// empty file where there had been none, which is the one outcome
// AtomicWriteFile promises never to produce.
func targetMode(path, probePath string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.Mode().Perm(), nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}

	// The cheapest way to learn what the umask allows is to let the kernel
	// apply it, in the directory the file is going to live in.
	probe, err := os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, DefaultFileMode)
	if err != nil {
		return ownerOnlyFileMode, nil
	}

	defer func() { _ = os.Remove(probePath) }()

	if err = probe.Close(); err != nil {
		return 0, fmt.Errorf("close %s: %w", probePath, err)
	}

	info, err = os.Stat(probePath)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", probePath, err)
	}

	return info.Mode().Perm(), nil
}

// AtomicWriteFile writes data to a sibling temp file, fsyncs it, then renames
// it over path. When path is a symlink the write goes to the resolved target
// so the link is preserved; new or dangling paths are written as-is.
func AtomicWriteFile(path string, data []byte) (err error) {
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = resolved
	}

	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}

	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write temp file %s: %w", tmp.Name(), err)
	}

	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("sync temp file %s: %w", tmp.Name(), err)
	}

	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmp.Name(), err)
	}

	// The umask probe borrows the temp file's name so that two writers racing
	// for the same destination cannot collide on it. targetMode removes it
	// before returning.
	mode, err := targetMode(path, tmp.Name()+".mode")
	if err != nil {
		return err
	}

	return renameIntoPlace(tmp.Name(), path, dir, mode)
}

// renameIntoPlace sets the temp file's mode, clears any read-only attribute
// on the destination (Windows only, see golang/go#38287), renames the temp
// over the target, and fsyncs the parent directory so the rename is durable.
// The mode passed in is what targetMode decided the file should end up with:
// the existing file's preserved mode on a replace, or umask-applied
// DefaultFileMode on a new file.
func renameIntoPlace(tmpPath, targetPath, dir string, mode os.FileMode) error {
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp file %s: %w", tmpPath, err)
	}

	// On Windows, os.Rename fails with "Access is denied" when the
	// destination is read-only (golang/go#38287). Clear the attribute first
	// so a read-only file is replaced the way it would be on POSIX. The chmod
	// above already set the temp file's mode, so the rename lands the right
	// permissions; targetMode returned the existing file's mode, which on
	// Windows is 0o444 for a read-only file, so the read-only attribute is
	// restored by that chmod.
	if err := clearReadOnlyBeforeRename(targetPath); err != nil {
		return fmt.Errorf("cannot clear read-only attribute on %s: %w", targetPath, err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, targetPath, err)
	}

	// Best-effort fsync of the parent directory so the rename is durable.
	// Silently no-op where unsupported (Windows).
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}

	return nil
}
