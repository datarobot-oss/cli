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

package fileops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// IgnoreFunc reports whether a normalized rel path should be excluded.
// Directories are queried so subtrees can be pruned at the root.
type IgnoreFunc func(relPath string, isDir bool) bool

// SymlinkLogger is called once per skipped symlink. nil disables it.
//
// isDir reports whether the link target resolves to a directory (resolved
// via os.Stat, which follows the link). A dangling symlink reports an empty
// target, isDir false, and dangling true: its target does not exist, so its
// kind is unknowable rather than "not a directory". Callers that filter by
// ignore patterns can use dangling to check both spellings of a pattern
// (file and directory) instead of only the file spelling.
type SymlinkLogger func(relPath, target string, isDir, dangling bool)

type Entry struct {
	AbsPath string
	RelPath string
}

// Walk enumerates regular files under root, deterministic by lexical
// order. Symlinks are never followed (would risk double-counting or
// escaping the project tree); each is reported via onSymlink. Ignored
// directories are pruned at the directory level.
func Walk(root string, ignore IgnoreFunc, onSymlink SymlinkLogger) ([]Entry, error) {
	var entries []Entry

	visit := func(path string, d fs.DirEntry, walkErr error) error {
		return walkVisit(root, ignore, onSymlink, &entries, path, d, walkErr)
	}

	if err := filepath.WalkDir(root, visit); err != nil {
		return nil, err
	}

	return entries, nil
}

func walkVisit(
	root string,
	ignore IgnoreFunc,
	onSymlink SymlinkLogger,
	entries *[]Entry,
	path string,
	d fs.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", path, walkErr)
	}

	if path == root {
		return nil
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relativize %s under %s: %w", path, root, err)
	}

	normRel := NormalizePath(rel)

	switch {
	case d.Type()&os.ModeSymlink != 0:
		notifySymlink(onSymlink, path, normRel)

		return nil
	case d.IsDir():
		return dirAction(normRel, ignore)
	case !d.Type().IsRegular():
		return nil
	}

	if ignore != nil && ignore(normRel, false) {
		return nil
	}

	*entries = append(*entries, Entry{AbsPath: path, RelPath: normRel})

	return nil
}

// notifySymlink resolves the link target and its kind, then delivers the
// callback. os.Stat follows the symlink, so a dangling link fails here and is
// reported with an empty target and isDir false — the user sees that a link
// points nowhere rather than getting a walk error. os.Readlink is still
// attempted first so a resolvable link reports the link text the user wrote;
// only when Stat fails (the target does not exist) is the target cleared.
func notifySymlink(onSymlink SymlinkLogger, absPath, relPath string) {
	if onSymlink == nil {
		return
	}

	target, _ := os.Readlink(absPath)

	// Stat follows the link. A failure means the target does not exist
	// (dangling) or is inaccessible. The two are kept apart: a dangling
	// link's kind is unknowable (dangling=true), while other Stat errors
	// keep the historical empty-target, isDir=false report.
	info, err := os.Stat(absPath)
	if err != nil {
		onSymlink(relPath, "", false, errors.Is(err, fs.ErrNotExist))

		return
	}

	onSymlink(relPath, target, info.IsDir(), false)
}

func dirAction(relPath string, ignore IgnoreFunc) error {
	if ignore != nil && ignore(relPath, true) {
		return filepath.SkipDir
	}

	return nil
}
