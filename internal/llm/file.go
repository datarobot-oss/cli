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

package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/workload"
)

// Payload formats the Workload API accepts.
const (
	FormatDynamoDGD   = "dynamo.dgd/v1beta1"
	FormatVLLMRecipe  = "vllm.recipe/v1"
	supportedFormats  = FormatDynamoDGD + ", " + FormatVLLMRecipe
	bareFileFormatKey = "format"
)

// RecipeFile is what a recipe file holds: either a full create request
// (name, description, descriptor, payload) or a bare payload (a top-level
// "format"), in which case only Payload is set.
type RecipeFile struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Descriptor  json.RawMessage `json:"descriptor,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// ReadRecipeFile reads a JSON or YAML recipe file and checks the payload's
// shape the way the Workload API does, so a malformed file fails locally.
func ReadRecipeFile(path string) (*RecipeFile, error) {
	raw, err := workload.ReadSpecFile(path)
	if err != nil {
		return nil, err
	}

	var top map[string]json.RawMessage

	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("recipe file %s must be a JSON or YAML object: %w", path, err)
	}

	var f RecipeFile

	if _, bare := top[bareFileFormatKey]; bare {
		f.Payload = raw
	} else if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("recipe file %s: %w", path, err)
	}

	if len(f.Payload) == 0 {
		return nil, fmt.Errorf("recipe file %s has no payload: give a full request with \"payload\", or a bare payload with \"format\"", path)
	}

	if err := ValidatePayload(f.Payload); err != nil {
		return nil, fmt.Errorf("recipe file %s: %w", path, err)
	}

	return &f, nil
}

// PayloadFormat is the payload's "format", or "" when it has none.
func PayloadFormat(payload json.RawMessage) string {
	var p struct {
		Format string `json:"format"`
	}

	_ = json.Unmarshal(payload, &p)

	return p.Format
}

// ValidatePayload mirrors the Workload API's payload check: a known format,
// a non-empty objects list for Dynamo, a recipe document for vLLM.
func ValidatePayload(payload json.RawMessage) error {
	var p struct {
		Format  string          `json:"format"`
		Objects []any           `json:"objects"`
		Recipe  json.RawMessage `json:"recipe"`
	}

	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload must be an object: %w", err)
	}

	switch p.Format {
	case FormatDynamoDGD:
		if len(p.Objects) == 0 {
			return fmt.Errorf("payload.objects must be a non-empty list of Kubernetes objects for %s", FormatDynamoDGD)
		}
	case FormatVLLMRecipe:
		if !strings.HasPrefix(strings.TrimSpace(string(p.Recipe)), "{") {
			return fmt.Errorf("payload.recipe must be a recipes.vllm.ai document for %s", FormatVLLMRecipe)
		}
	default:
		return fmt.Errorf("payload.format must be one of %s (got %q)", supportedFormats, p.Format)
	}

	return nil
}
