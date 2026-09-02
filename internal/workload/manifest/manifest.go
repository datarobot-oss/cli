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

package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"gopkg.in/yaml.v3"
)

// FileName is the manifest's file name, conventionally at the repository root.
const FileName = ".datarobot.yaml"

// ErrNotFound reports that Locate found no manifest. Callers branch on it
// with errors.Is, e.g. to fall into the setup wizard instead of failing.
var ErrNotFound = errors.New("no " + FileName + " manifest found")

// ErrEmptyFile reports a manifest with nothing in it. Named with its remedy
// because the likeliest way to get here is a write that was killed between
// reserving the path and filling it. The reservation is what stops two setups
// racing, so it is kept, and what it can leave behind is a file the user has to
// be told to delete rather than one they are left guessing at.
//
// Shared rather than phrased at each site: the same file reaches the reader
// through Parse and the write-back through editRoot, and two explanations of
// one state, differing by which function happened to touch the file first, is
// one explanation too many.
var ErrEmptyFile = errors.New(
	"file is empty, which an interrupted write can leave behind: delete it and run the command again")

// ErrNotADirectory reports that the path a search was told to start from is
// not a directory. It is deliberately not ErrNotFound: nothing was searched,
// so falling back to whatever a missing manifest triggers would act on a
// directory the caller never named.
var ErrNotADirectory = errors.New("not a directory")

// recognizedKeys are the top-level keys that identify a workload manifest. A
// file with none of them is rejected as foreign rather than deployed as an
// all-defaults workload.
var recognizedKeys = []string{keyWorkloadID, keyName, keyImportance, keyArtifact, keyArtifactID, keyRuntime}

// Manifest is a parsed .datarobot.yaml. Validation and compilation both
// derive from the one retained parse tree, so they cannot disagree about
// what the file says.
type Manifest struct {
	// Path is the absolute path the manifest was loaded from, "" when it was
	// parsed from bytes.
	Path string
	// Dir anchors file-existence rules such as the provided-mode Dockerfile
	// check; "" skips them.
	Dir string

	root *yaml.Node
}

// Load reads and parses the manifest at path. The file's directory becomes
// Dir, anchoring file-existence validation rules.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("manifest not found: %s", path)
		}

		return nil, fmt.Errorf("cannot read manifest %s: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve manifest path %s: %w", path, err)
	}

	m, err := Parse(data, filepath.Dir(abs))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	m.Path = abs

	return m, nil
}

// Parse parses manifest bytes. dir anchors file-existence validation rules;
// pass "" to skip them, e.g. when previewing content that is not on disk
// yet. JSON content parses too: the manifest format is whatever the create
// endpoint accepts, and YAML is a superset of JSON.
func Parse(data []byte, dir string) (*Manifest, error) {
	var doc yaml.Node

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}

	if root.Kind == 0 {
		return nil, ErrEmptyFile
	}

	if root.Kind != yaml.MappingNode {
		return nil, errors.New("root must be a YAML mapping")
	}

	// Before anything walks the tree, including the check just below.
	if err := checkAliases(root); err != nil {
		return nil, err
	}

	if !hasRecognizedKey(root) {
		return nil, fmt.Errorf("does not look like a DataRobot workload manifest (none of %s present)",
			strings.Join(recognizedKeys, ", "))
	}

	return &Manifest{Dir: dir, root: root}, nil
}

// DirFlag is the " --dir <path>" a remedy has to carry to reach the project in
// dir, and empty when the working directory already does.
//
// One implementation, because two commands print this suffix at each other: a
// message that names 'dr workload delete <id>' without it is false in exactly
// the layout --dir was added for, and a message that adds it when the shell is
// already standing in the project tells the reader to type something that does
// nothing. Two notions of "already there" would eventually disagree about one
// directory, and the disagreement would be between the two commands this
// binding is passed between.
//
// The path is resolved rather than compared as text, so "." , "./." and a
// trailing slash all answer the same way. A directory that cannot be resolved
// answers empty: a suffix guessed from a path the OS would not confirm is worse
// than none.
func DirFlag(dir string) string {
	if dir == "" {
		return ""
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	project, err := filepath.Abs(dir)
	if err != nil || project == cwd {
		return ""
	}

	// Relative when it is shorter to read and still lands in the same place;
	// absolute otherwise, which is what a project on another branch of the tree
	// gets rather than a ../../.. chain.
	//
	// Forward slashes on every platform. The CLI accepts them on Windows too,
	// since resolving --dir goes through filepath.Abs, which normalizes them;
	// what it would not survive is a backslash pasted into a POSIX shell, where
	// it is an escape rather than a separator, and quoting it away would take
	// quotes cmd.exe does not read.
	if rel, err := filepath.Rel(cwd, project); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		return " --dir " + shellArg(filepath.ToSlash(rel))
	}

	return " --dir " + shellArg(filepath.ToSlash(project))
}

// shellArg quotes a path that would not survive being pasted into a shell.
//
// These suffixes are printed inside commands the reader is meant to copy and
// run, and a path is not a word: `/tmp/a&b` backgrounds the command at the
// ampersand, `$HOME` and a backtick are expanded before the command ever sees
// them, and a space simply ends the argument.
//
// An allowlist rather than a list of the characters that bite, because the
// list of what a shell does something with is long, differs between shells,
// and grows. Anything outside the set below is quoted; a path that is only
// letters, digits and the handful of punctuation marks a path normally
// carries is left bare, so the ordinary message is not littered with quotes.
//
// Single quotes, because inside them a shell expands nothing at all. The one
// character they cannot carry is a single quote, which is closed, escaped and
// reopened in the usual way.
func shellArg(path string) string {
	if path != "" && !strings.ContainsFunc(path, needsQuoting) {
		return path
	}

	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// needsQuoting reports a character a shell would not hand over as itself.
func needsQuoting(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case strings.ContainsRune("._-+/:@,=", r):
		return false
	default:
		return true
	}
}

// Locate searches for the manifest in startDir and each ancestor, the way
// git finds its repository root. The walk is lexical: symlinks are not
// resolved, and it stops after checking the user's home directory or the
// filesystem root, whichever comes first. Returns ErrNotFound when no
// manifest exists on the walk.
func Locate(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", startDir, err)
	}

	// Checked here rather than at each caller, because the hazard belongs to
	// the walk: a directory that is not there has no manifest of its own, so
	// the loop below sails past it to an ancestor and answers with a different
	// project's file. That is a wrong answer, not a missing one, so it is not
	// ErrNotFound: a caller that falls back to the setup wizard on ErrNotFound
	// would otherwise configure the parent of the directory it was pointed at.
	if !fsutil.DirExists(dir) {
		return "", fmt.Errorf("%w: %s", ErrNotADirectory, startDir)
	}

	home, _ := os.UserHomeDir()

	for {
		candidate := filepath.Join(dir, FileName)
		if fsutil.FileExists(candidate) {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if dir == home || parent == dir {
			return "", fmt.Errorf("%w in %s or any parent directory", ErrNotFound, startDir)
		}

		dir = parent
	}
}

// WorkloadID returns the CLI-managed workload binding, "" when the manifest
// is not yet bound to a workload.
func (m *Manifest) WorkloadID() string {
	return topLevelString(m.root, keyWorkloadID)
}

// Name returns the workload name, "" when absent.
func (m *Manifest) Name() string {
	return topLevelString(m.root, keyName)
}

// fileLabel names the manifest in error messages: the real path when the
// manifest came from disk, the conventional file name otherwise.
func (m *Manifest) fileLabel() string {
	if m.Path != "" {
		return m.Path
	}

	return FileName
}

func hasRecognizedKey(root *yaml.Node) bool {
	for _, key := range recognizedKeys {
		if mapValue(root, key) != nil {
			return true
		}
	}

	return false
}

func topLevelString(root *yaml.Node, key string) string {
	value, _ := scalarString(mapValue(root, key))

	return value
}
