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

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/datarobot/cli/internal/repo"
	"gopkg.in/yaml.v3"
)

func GetRequirements() ([]Prerequisite, []string, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		return nil, nil, err
	}

	return GetRequirementsFromDir(filepath.Join(repoRoot, ".datarobot", "cli"))
}

// GetRequirementsFromDir reads prerequisites from a versions.yaml file in the given directory.
// Used by plugin execution to load plugin-specific dependency requirements.
func GetRequirementsFromDir(dir string) ([]Prerequisite, []string, error) {
	yamlFile := filepath.Join(dir, "versions.yaml")

	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to read versions yaml file %s: %w", yamlFile, err)
	}

	// Decoded via yaml.Node (not a map[string]Prerequisite) to preserve the file's
	// declaration order: dependency installation runs in this order, and tools like
	// pulumi-datarobot's install command need pulumi already installed, so map
	// iteration's randomized order would install prerequisites out of order.
	var root yaml.Node

	if err = yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("Failed to unmarshal versions yaml file %s: %w", yamlFile, err)
	}

	if len(root.Content) == 0 {
		return nil, nil, nil
	}

	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("versions yaml file %s must contain a top-level mapping", yamlFile)
	}

	var violations []string

	versions := make([]Prerequisite, 0, len(mapping.Content)/2)
	seen := make(map[string]bool, len(mapping.Content)/2)

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value

		// yaml.Node traversal doesn't reject duplicate keys the way decoding into a
		// map does, so check explicitly to keep that same fail-fast behavior.
		if seen[key] {
			return nil, nil, fmt.Errorf("versions yaml file %s: duplicate key %q (line %d)", yamlFile, key, mapping.Content[i].Line)
		}

		seen[key] = true

		var version Prerequisite
		if err := mapping.Content[i+1].Decode(&version); err != nil {
			return nil, nil, fmt.Errorf("Failed to unmarshal entry %q in %s: %w", key, yamlFile, err)
		}

		version.Key = key
		violations = append(violations, validatePrerequisite(key, version)...)
		versions = append(versions, version)
	}

	return versions, violations, nil
}

func GetSelfRequirement() (Prerequisite, error) {
	prerequisites, _, err := GetRequirements()
	if err != nil {
		return Prerequisite{}, nil
	}

	selfIndex := slices.IndexFunc(prerequisites, func(p Prerequisite) bool {
		return p.Key == "dr"
	})

	if selfIndex == -1 {
		return Prerequisite{}, nil
	}

	return prerequisites[selfIndex], nil
}
