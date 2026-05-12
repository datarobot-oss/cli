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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const rollbackDirName = ".rollback"

// Rollback owns the .wapi/.rollback/ tree for a single sync run. Call
// Backup before each destructive operation, then Discard on success or
// Restore on failure. Newly created files go through TrackCreated so
// Restore can remove them.
type Rollback struct {
	projectDir string
	rollDir    string
	created    []string
}

// NewRollback creates the rollback directory. Returns an error if a stale
// rollback already exists; callers must run RestoreStaleIfPresent first.
func NewRollback(projectDir string) (*Rollback, error) {
	rollDir := filepath.Join(projectDir, ".wapi", rollbackDirName)

	if err := os.Mkdir(rollDir, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("rollback dir already exists: %s", rollDir)
		}

		return nil, fmt.Errorf("create rollback dir: %w", err)
	}

	return &Rollback{projectDir: projectDir, rollDir: rollDir}, nil
}

// Backup copies relPath into the rollback tree. Missing files are OK;
// the absence is implicitly recorded.
func (r *Rollback) Backup(relPath string) error {
	src := filepath.Join(r.projectDir, filepath.FromSlash(relPath))

	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("stat for backup %s: %w", src, err)
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	dst := filepath.Join(r.rollDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir rollback parent: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source for backup: %w", err)
	}

	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy backup file: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close backup file: %w", err)
	}

	return nil
}

// TrackCreated records a file that Restore should remove.
func (r *Rollback) TrackCreated(absPath string) {
	r.created = append(r.created, absPath)
}

// Discard removes the rollback directory after a successful sync.
func (r *Rollback) Discard() error {
	if err := os.RemoveAll(r.rollDir); err != nil {
		return fmt.Errorf("remove rollback dir: %w", err)
	}

	return nil
}

// Restore copies every backup back into the project directory and
// removes files tracked in r.created.
func (r *Rollback) Restore() error {
	for _, abs := range r.created {
		_ = os.Remove(abs)
	}

	return restoreFromDir(r.rollDir, r.projectDir)
}

// RestoreStaleIfPresent restores any existing .wapi/.rollback/ tree and
// removes it. The bool indicates whether a restore happened so callers
// can warn the user.
func RestoreStaleIfPresent(projectDir string) (bool, error) {
	rollDir := filepath.Join(projectDir, ".wapi", rollbackDirName)

	info, err := os.Stat(rollDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("stat rollback dir: %w", err)
	}

	if !info.IsDir() {
		return false, fmt.Errorf("%s exists but is not a directory", rollDir)
	}

	if err := restoreFromDir(rollDir, projectDir); err != nil {
		return true, err
	}

	if err := os.RemoveAll(rollDir); err != nil {
		return true, fmt.Errorf("remove rollback dir after restore: %w", err)
	}

	return true, nil
}

func restoreFromDir(rollDir, projectDir string) error {
	return filepath.WalkDir(rollDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk rollback %s: %w", p, walkErr)
		}

		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(rollDir, p)
		if err != nil {
			return fmt.Errorf("relativize rollback %s: %w", p, err)
		}

		dst := filepath.Join(projectDir, rel)

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir restore parent: %w", err)
		}

		in, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open backup %s: %w", p, err)
		}

		out, err := os.Create(dst)
		if err != nil {
			_ = in.Close()
			return fmt.Errorf("create restored file %s: %w", dst, err)
		}

		_, copyErr := io.Copy(out, in)

		_ = in.Close()
		_ = out.Close()

		if copyErr != nil {
			return fmt.Errorf("restore copy %s: %w", dst, copyErr)
		}

		return nil
	})
}
