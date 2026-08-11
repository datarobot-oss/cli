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
	"fmt"
	"os"
	"path/filepath"
)

// DefaultFileMode is the permission CLI-written data files get: readable by
// anyone who can read the project, writable by the owner. Exported so callers
// that open a file themselves (an append-only log, say) stay consistent with
// what AtomicWriteFile leaves behind.
const DefaultFileMode os.FileMode = 0o644

// targetMode is the permission the written file should end up with.
//
// Replacing a file keeps the mode it already had: a user who tightened
// something to 0600 should not find it widened by an unrelated edit. A new
// file gets what a plain create would give it, umask included, because
// os.CreateTemp makes the temp file 0600 and chmodding to a fixed mode would
// hand a developer with a restrictive umask a wider file than everything else
// they write.
func targetMode(path string) (os.FileMode, error) {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm(), nil
	}

	// The cheapest way to learn what the umask allows is to let the kernel
	// apply it, in the directory the file is going to live in.
	probe, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, DefaultFileMode)
	if err != nil {
		// Something else created it in between, or the directory refuses; the
		// rename below reports whichever it is.
		return DefaultFileMode, nil
	}

	_ = probe.Close()

	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}

	return info.Mode().Perm(), nil
}

// AtomicWriteFile writes data to a sibling temp file in the same directory as
// path, fsyncs it, then renames it over path. os.Rename is atomic on POSIX
// and handled as replace-on-existing by Go on Windows. The parent directory
// is fsynced after rename so the new dentry survives a crash. The temp file
// is removed on any failure before rename so no .tmp.* leftovers remain.
func AtomicWriteFile(path string, data []byte) (err error) {
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

	mode, err := targetMode(path)
	if err != nil {
		return err
	}

	if err = os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("chmod temp file %s: %w", tmp.Name(), err)
	}

	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp.Name(), path, err)
	}

	// Best-effort fsync of the parent directory so the rename is durable.
	// Silently no-op where unsupported (Windows).
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}

	return nil
}
