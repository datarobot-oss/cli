// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package copier

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
)

type Answers struct {
	FileName         string
	ComponentDetails Details
	// TODO: Add more properties to account for what we need to determine as canonical values expected for components

	Repo string `yaml:"_src_path"`
}

func AnswersFromPath(path string, all bool) ([]Answers, error) {
	pattern := filepath.Join(path, ".datarobot/answers/*.y*ml")

	yamlFiles, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	result := make([]Answers, 0)

	for _, yamlFile := range yamlFiles {
		data, err := os.ReadFile(yamlFile)
		if err != nil {
			log.Errorf("Failed to read yaml file %s: %s", yamlFile, err)
			continue
		}

		fileParsed := Answers{FileName: yamlFile}

		if err = yaml.Unmarshal(data, &fileParsed); err != nil {
			log.Errorf("Failed to unmarshal yaml file %s: %s", yamlFile, err)
			continue
		}

		componentDetails := ComponentDetailsByURL[fileParsed.Repo]

		if all || componentDetails.Enabled {
			fileParsed.ComponentDetails = componentDetails
			result = append(result, fileParsed)
		}
	}

	return result, nil
}
