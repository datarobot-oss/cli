// Copyright 2025 DataRobot, Inc. and its affiliates.
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
