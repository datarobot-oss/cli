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

package appframework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ComponentInstance represents a single installed module instance in a project.
type ComponentInstance struct {
	Label   string                 // e.g. "core.agent.1"
	Module  string                 // e.g. "core.agent"
	Answers map[string]interface{} // disambiguated question name -> value
}

// instanceStateResponse is an internal type for JSON unmarshalling of `describe` output.
// Mirrors InstanceState in models/core_types.py.
type instanceStateResponse struct {
	Labels  map[string]string                 `json:"labels"`  // label -> disambiguated module name
	Answers map[string]map[string]interface{} `json:"answers"` // label -> answers map
}

// ListInstalled runs dr-app-framework describe and returns all installed component instances.
// Returns an empty slice (no error) if no instance state file exists yet.
// The slice is sorted by label for stable TUI ordering.
func ListInstalled(fw, target string) ([]ComponentInstance, error) {
	// Fast path: if the instance state file doesn't exist, the project has no installed components.
	stateFile := filepath.Join(target, ".datarobot", "af-instance-state.yml")

	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return []ComponentInstance{}, nil
	}

	data, err := cmdOutput(afCommand("describe", "-f", fw, "-t", target))
	if err != nil {
		return nil, fmt.Errorf("describe: %w", err)
	}

	var resp instanceStateResponse

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing describe response: %w", err)
	}

	instances := make([]ComponentInstance, 0, len(resp.Labels))

	for label, moduleName := range resp.Labels {
		answers := resp.Answers[label]
		if answers == nil {
			answers = map[string]interface{}{}
		}

		instances = append(instances, ComponentInstance{
			Label:   label,
			Module:  moduleName,
			Answers: answers,
		})
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Label < instances[j].Label
	})

	return instances, nil
}
