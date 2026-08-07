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
		return nil, errors.New("file is empty")
	}

	if root.Kind != yaml.MappingNode {
		return nil, errors.New("root must be a YAML mapping")
	}

	if !hasRecognizedKey(root) {
		return nil, fmt.Errorf("does not look like a DataRobot workload manifest (none of %s present)",
			strings.Join(recognizedKeys, ", "))
	}

	return &Manifest{Dir: dir, root: root}, nil
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
