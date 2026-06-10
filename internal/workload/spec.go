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

package workload

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ReadSpecFile reads a workload or artifact spec from path and returns it as
// a JSON payload. Valid JSON is passed through byte-for-byte; anything else
// is parsed as YAML and converted, because the Workload API accepts only
// JSON. Format is detected from the content, not the file extension.
func ReadSpecFile(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("file not found: %s", path)
		}

		return nil, err
	}

	if json.Valid(data) {
		return json.RawMessage(data), nil
	}

	var doc any

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("spec file %s is neither valid JSON nor valid YAML: %w", path, err)
	}

	converted, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot convert YAML spec %s to JSON: %w", path, err)
	}

	return json.RawMessage(converted), nil
}
