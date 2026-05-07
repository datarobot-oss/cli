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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// IgnoreFunc reports whether a normalized rel path should be excluded.
// Directories are queried so subtrees can be pruned at the root.
type IgnoreFunc func(relPath string, isDir bool) bool

// SymlinkLogger is called once per skipped symlink. nil disables it.
type SymlinkLogger func(relPath, target string)

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

func notifySymlink(onSymlink SymlinkLogger, absPath, relPath string) {
	if onSymlink == nil {
		return
	}

	target, _ := os.Readlink(absPath)
	onSymlink(relPath, target)
}

func dirAction(relPath string, ignore IgnoreFunc) error {
	if ignore != nil && ignore(relPath, true) {
		return filepath.SkipDir
	}

	return nil
}
