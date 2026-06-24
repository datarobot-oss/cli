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

type versionsYaml map[string]Prerequisite

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

	var fileParsed versionsYaml

	if err = yaml.Unmarshal(data, &fileParsed); err != nil {
		return nil, nil, fmt.Errorf("Failed to unmarshal versions yaml file %s: %w", yamlFile, err)
	}

	violations := versionsYamlSchema.Validate(fileParsed)

	versions := make([]Prerequisite, 0, len(fileParsed))

	for key, version := range fileParsed {
		version.Key = key

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
