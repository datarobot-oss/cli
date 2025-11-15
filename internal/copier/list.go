// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package copier

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Answers struct {
	FileName string
	Repo     string `yaml:"_src_path"`
}

// TODO: Add more properties to account for what we need to determine as canonical values expected for components
type Component struct {
	FileName string
	SrcPath  string `yaml:"_src_path"`
}

func AnswersFromPath(path string) ([]Answers, error) {
	pattern := filepath.Join(path, ".datarobot/answers/*.y*ml")

	yamlFiles, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	result := make([]Answers, 0)

	for _, yamlFile := range yamlFiles {
		data, err := os.ReadFile(yamlFile)
		if err != nil {
			return nil, fmt.Errorf("Failed to read yaml file %s: %w", yamlFile, err)
		}

		fileParsed := Answers{FileName: yamlFile}

		if err = yaml.Unmarshal(data, &fileParsed); err != nil {
			return nil, fmt.Errorf("Failed to unmarshal yaml file %s: %w", yamlFile, err)
		}

		result = append(result, fileParsed)
	}

	return result, nil
}

func ComponentsFromAnswers(answers []Answers) ([]Component, error) {
	components := make([]Component, 0)

	for _, a := range answers {
		data, err := os.ReadFile(a.FileName)
		if err != nil {
			return nil, fmt.Errorf("Failed to read yaml file %s: %w", a.FileName, err)
		}

		component := Component{FileName: a.FileName}

		if err = yaml.Unmarshal(data, &component); err != nil {
			return nil, fmt.Errorf("Failed to unmarshal yaml file %s: %w", a.FileName, err)
		}

		components = append(components, component)
	}

	return components, nil
}
