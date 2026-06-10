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

// Question mirrors the Question type in the dr-app-framework Python CLI.
// JSON keys match models/core_types.py:Question.model_dump_json().
type Question struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"display_name"`
	Help        string        `json:"help"`
	Default     interface{}   `json:"default"`
	Type        string        `json:"type"` // str | int | bool | json | yaml
	Choices     []interface{} `json:"choices"`
}

// Module mirrors the Module type in the dr-app-framework Python CLI.
// DisambiguatedName is populated from the modules map key (e.g. "core.agent"), not a JSON field.
type Module struct {
	DisambiguatedName string     // set from the describe-framework modules map key
	Name              string     `json:"name"`
	Registry          string     `json:"registry"`
	DisplayName       string     `json:"display_name"`
	Description       string     `json:"description"`
	Tags              []string   `json:"tags"`
	Questions         []Question `json:"questions"`
}

// describeFrameworkResponse is an internal type for JSON unmarshalling of describe-framework output.
type describeFrameworkResponse struct {
	Registries map[string]struct {
		Alias string `json:"alias"`
	} `json:"registries"`
	Modules map[string]Module `json:"modules"`
}

// DescribeFramework runs dr-app-framework describe-framework and returns the available modules.
// The slice is sorted by DisplayName for stable TUI ordering.
// Returns an empty slice (no error) if the framework has not been initialized yet.
func DescribeFramework(fw, target string) ([]Module, error) {
	// Fast path: if framework.yml doesn't exist the framework hasn't been initialized.
	fwYML := filepath.Join(fw, "framework.yml")

	if _, err := os.Stat(fwYML); os.IsNotExist(err) {
		return []Module{}, nil
	}

	data, err := cmdOutput(afCommand("describe-framework", "-f", fw, "-t", target))
	if err != nil {
		return nil, fmt.Errorf("describe-framework: %w", err)
	}

	var resp describeFrameworkResponse

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing describe-framework response: %w", err)
	}

	modules := make([]Module, 0, len(resp.Modules))

	for disambigName, m := range resp.Modules {
		m.DisambiguatedName = disambigName

		if m.DisplayName == "" {
			m.DisplayName = m.Name
		}

		modules = append(modules, m)
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].DisplayName < modules[j].DisplayName
	})

	return modules, nil
}

// RegistryAliases runs describe-framework and returns the set of registered registry aliases.
// Returns an empty set (no error) if the framework hasn't been initialized.
func RegistryAliases(fw, target string) (map[string]bool, error) {
	fwYML := filepath.Join(fw, "framework.yml")

	if _, err := os.Stat(fwYML); os.IsNotExist(err) {
		return map[string]bool{}, nil
	}

	data, err := cmdOutput(afCommand("describe-framework", "-f", fw, "-t", target))
	if err != nil {
		return nil, fmt.Errorf("describe-framework: %w", err)
	}

	var resp describeFrameworkResponse

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing describe-framework response: %w", err)
	}

	aliases := make(map[string]bool, len(resp.Registries))

	for alias := range resp.Registries {
		aliases[alias] = true
	}

	return aliases, nil
}
