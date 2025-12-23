// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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

func GetRequirements() ([]Prerequisite, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		return nil, err
	}

	if repoRoot == "" {
		return nil, nil
	}

	yamlFile := filepath.Join(repoRoot, ".datarobot", "cli", "versions.yaml")

	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("Failed to read versions yaml file %s: %w", yamlFile, err)
	}

	var fileParsed versionsYaml

	if err = yaml.Unmarshal(data, &fileParsed); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal versions yaml file %s: %w", yamlFile, err)
	}

	versions := make([]Prerequisite, 0, len(fileParsed))

	for key, version := range fileParsed {
		version.Key = key

		versions = append(versions, version)
	}

	return versions, nil
}

func GetSelfRequirement() (Prerequisite, error) {
	prerequisites, err := GetRequirements()
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
