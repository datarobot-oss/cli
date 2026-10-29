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
	"slices"
	"strings"
)

// IgnoreFunc reports whether a normalized rel path should be excluded.
// Directories are queried so subtrees can be pruned at the root.
type IgnoreFunc func(relPath string, isDir bool) bool

// LinkOutcome is what the walk did with a link.
type LinkOutcome string

const (
	// LinkFollowed: what the link points at was walked in its place.
	LinkFollowed LinkOutcome = "followed"
	// LinkDangling: the link points at nothing.
	LinkDangling LinkOutcome = "dangling"
	// LinkOutside: the link points outside the walked root.
	LinkOutside LinkOutcome = "outside_project"
	// LinkLoop: the link points at a directory the walk is already inside.
	LinkLoop LinkOutcome = "loop"
	// LinkTargetIgnored: the ignore rules exclude what the link points at.
	LinkTargetIgnored LinkOutcome = "target_ignored"
	// LinkUnreadable: the link's target could not be resolved.
	LinkUnreadable LinkOutcome = "unreadable"
)

// Link is a symlink, or on Windows a junction, that the walk met.
//
// IsDir reports whether the link resolves to a directory. A link that
// resolves to nothing reports IsDir false: its kind is unknowable rather than
// "not a directory".
type Link struct {
	RelPath string
	// Target is the link text as written, empty when the link resolves to
	// nothing.
	Target  string
	IsDir   bool
	Outcome LinkOutcome
}

// LinkFunc is called once per link the walk meets, followed or not. A link
// the ignore function excludes is not reported. nil disables it.
type LinkFunc func(Link)

type Entry struct {
	AbsPath string
	RelPath string
}

// Walk enumerates regular files under root, deterministic by lexical order.
// Ignored directories are pruned at the directory level.
//
// A link whose target lies inside root is walked as if its target sat at the
// link's path, so a linked file or directory is listed under the link's name
// as well as its own. A link pointing outside root, at nothing, or back into
// a directory the walk is inside is not followed. Each link is reported via
// onLink with what was done with it.
func Walk(root string, ignore IgnoreFunc, onLink LinkFunc) ([]Entry, error) {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", root, err)
	}

	w := &walker{rootReal: rootReal, ignore: ignore, onLink: onLink}

	if err := w.walk(root, "", nil); err != nil {
		return nil, err
	}

	return w.entries, nil
}

type walker struct {
	rootReal string
	ignore   IgnoreFunc
	onLink   LinkFunc
	entries  []Entry
}

// walk lists dir, which sits at prefix relative to the root ("" for the root
// itself). inside holds the real directories the walk passed through to get
// here, so that a link back into one of them is caught as a loop.
func (w *walker) walk(dir, prefix string, inside []string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", path, walkErr)
		}

		if path == dir {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relativize %s under %s: %w", path, dir, err)
		}

		normRel := NormalizePath(rel)
		if prefix != "" {
			normRel = prefix + "/" + normRel
		}

		switch {
		case isLink(path, d.Type()):
			return w.link(path, normRel, inside)
		case d.IsDir():
			return dirAction(normRel, w.ignore)
		case !d.Type().IsRegular():
			return nil
		}

		if w.ignored(normRel, false) {
			return nil
		}

		w.entries = append(w.entries, Entry{AbsPath: path, RelPath: normRel})

		return nil
	})
}

// link follows the link at path, listed as rel, when its target is inside
// the root and walking it cannot loop, and reports what it did.
func (w *walker) link(path, rel string, inside []string) error {
	resolved, info, err := resolveLink(path)
	if err != nil {
		w.unresolved(rel, err)

		return nil
	}

	if w.ignored(rel, info.IsDir()) {
		return nil
	}

	// The directory the link sits in is one the walk is inside.
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		inside = append(slices.Clone(inside), dir)
	}

	target, _ := os.Readlink(path)
	link := Link{RelPath: rel, Target: target, IsDir: info.IsDir(), Outcome: w.outcome(resolved, info.IsDir(), inside)}

	w.report(link)

	switch {
	case link.Outcome != LinkFollowed:
		return nil
	case info.IsDir():
		return w.walk(resolved, rel, append(slices.Clone(inside), resolved))
	case info.Mode().IsRegular():
		w.entries = append(w.entries, Entry{AbsPath: resolved, RelPath: rel})
	}

	return nil
}

// outcome decides whether to follow a link that resolves to resolved.
func (w *walker) outcome(resolved string, isDir bool, inside []string) LinkOutcome {
	switch {
	case !within(w.rootReal, resolved):
		return LinkOutside
	case w.targetIgnored(resolved, isDir):
		return LinkTargetIgnored
	case isDir && loops(resolved, inside):
		return LinkLoop
	}

	return LinkFollowed
}

// unresolved reports a link that resolves to nothing, err saying why.
func (w *walker) unresolved(rel string, err error) {
	// A dangling link's kind is unknowable, so either spelling of an ignore
	// pattern excludes it.
	dangling := errors.Is(err, fs.ErrNotExist)
	if w.ignored(rel, false) || (dangling && w.ignored(rel, true)) {
		return
	}

	outcome := LinkUnreadable
	if dangling {
		outcome = LinkDangling
	}

	w.report(Link{RelPath: rel, Outcome: outcome})
}

func (w *walker) ignored(rel string, isDir bool) bool {
	return w.ignore != nil && w.ignore(rel, isDir)
}

// targetIgnored reports whether the ignore rules exclude resolved, a path
// inside the root, or a directory above it. Following such a link would upload
// what the project excludes under another name.
func (w *walker) targetIgnored(resolved string, isDir bool) bool {
	rel, err := filepath.Rel(w.rootReal, resolved)
	if err != nil {
		return true
	}

	rel = NormalizePath(rel)
	if rel == "." {
		return false
	}

	parts := strings.Split(rel, "/")

	for i := 1; i < len(parts); i++ {
		if w.ignored(strings.Join(parts[:i], "/"), true) {
			return true
		}
	}

	return w.ignored(rel, isDir)
}

func (w *walker) report(link Link) {
	if w.onLink != nil {
		w.onLink(link)
	}
}

// isLink reports whether the entry at path is a symlink or a junction. Go
// reports a Windows junction as irregular rather than as a symlink, so an
// irregular entry counts when it reads as a link.
func isLink(path string, mode fs.FileMode) bool {
	if mode&fs.ModeSymlink != 0 {
		return true
	}

	if mode&fs.ModeIrregular == 0 {
		return false
	}

	_, err := os.Readlink(path)

	return err == nil
}

// maxLinkHops bounds a chain of links, as the OS does for symlinks.
const maxLinkHops = 40

// resolveLink returns the real path of what the link at path points at,
// through any chain of links, with that target's info. EvalSymlinks alone
// does not do on Windows, where it leaves junctions in place.
func resolveLink(path string) (string, fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}

	current := path

	for range maxLinkHops {
		target, err := os.Readlink(current)
		if err != nil {
			return "", nil, err
		}

		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}

		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return "", nil, err
		}

		lstat, err := os.Lstat(resolved)
		if err != nil {
			return "", nil, err
		}

		if !isLink(resolved, lstat.Mode().Type()) {
			return resolved, info, nil
		}

		current = resolved
	}

	return "", nil, fmt.Errorf("resolve %s: too many links", path)
}

// loops reports whether walking the directory resolved would revisit one the
// walk is already inside.
func loops(resolved string, inside []string) bool {
	return slices.ContainsFunc(inside, func(dir string) bool { return within(resolved, dir) })
}

// within reports whether path is dir or lies under it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)

	return err == nil && !filepath.IsAbs(rel) && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func dirAction(relPath string, ignore IgnoreFunc) error {
	if ignore != nil && ignore(relPath, true) {
		return filepath.SkipDir
	}

	return nil
}
