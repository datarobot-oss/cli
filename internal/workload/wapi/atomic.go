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

package wapi

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicFileMode is the permission we always write .wapi/ state files
// with — and also the root-level .wapiignore template drop in Initialize.
const atomicFileMode os.FileMode = 0o644

// atomicWriteFile writes data to a sibling temp file in the same directory as
// path, fsyncs it, then renames it over path. os.Rename is atomic on POSIX
// and handled as replace-on-existing by Go on Windows. The parent directory
// is fsynced after rename so the new dentry survives a crash. The temp file
// is removed on any failure before rename so no .tmp.* leftovers remain.
func atomicWriteFile(path string, data []byte) (err error) {
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

	if err = os.Chmod(tmp.Name(), atomicFileMode); err != nil {
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
