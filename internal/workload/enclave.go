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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Enclave selection policies as serialized by the server. Enclave placement
// is opt-in per workload: "availability" lets DataRobot pick among the
// Enclaves granted to the workload's Use Case; "manual" pins the workload to
// the Enclave named in runtime.enclaves. Without a policy the workload is
// not placed on an Enclave.
const (
	EnclaveSelectionPolicyAvailability = "availability"
	EnclaveSelectionPolicyManual       = "manual"
)

// ApplyEnclavePin pins a JSON workload create spec to a single Enclave:
// runtime.enclaveSelectionPolicy becomes "manual" and runtime.enclaves the
// one-element list the server accepts today. It errors if the spec already
// sets either field rather than silently rewriting it. Re-encodes the spec:
// numbers survive via json.Number, key order does not. Returns
// json.RawMessage, not []byte, so handing the result to a marshalling
// client sends JSON rather than a base64 string.
func ApplyEnclavePin(spec []byte, enclave string) (json.RawMessage, error) {
	name := strings.TrimSpace(enclave)
	if name == "" {
		return nil, errors.New("invalid --enclave: the Enclave name must be non-blank")
	}

	doc, err := decodeSpecObject(spec)
	if err != nil {
		return nil, err
	}

	runtime := map[string]any{}

	if raw, ok := doc["runtime"]; ok && raw != nil {
		runtime, ok = raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid spec: 'runtime' must be an object")
		}
	}

	if _, ok := runtime["enclaveSelectionPolicy"]; ok {
		return nil, errors.New(
			"spec already sets runtime.enclaveSelectionPolicy; remove it from the spec or drop --enclave")
	}

	if _, ok := runtime["enclaves"]; ok {
		return nil, errors.New(
			"spec already sets runtime.enclaves; remove it from the spec or drop --enclave")
	}

	runtime["enclaveSelectionPolicy"] = EnclaveSelectionPolicyManual
	runtime["enclaves"] = []string{name}
	doc["runtime"] = runtime

	return json.Marshal(doc)
}

// decodeSpecObject decodes a JSON spec into a map, preserving numbers via
// json.Number, and rejects anything that is not a JSON object.
func decodeSpecObject(spec []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(spec))
	dec.UseNumber()

	var doc map[string]any

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	if doc == nil {
		return nil, errors.New("invalid spec: the spec must be a JSON object")
	}

	return doc, nil
}

// ApplyUseCase links a workload create spec to a Use Case: the top-level
// useCaseId field the server reads at create time. Enclave placement is
// opt-in per workload and always governed by a Use Case, so when neither
// the spec nor an already-applied pin has chosen an enclaveSelectionPolicy,
// the policy becomes "availability" and DataRobot picks among the Enclaves
// granted to the Use Case. It errors if the spec already sets useCaseId
// rather than silently rewriting it. Re-encodes the spec the same way
// ApplyEnclavePin does.
func ApplyUseCase(spec []byte, useCaseID string) (json.RawMessage, error) {
	id := strings.TrimSpace(useCaseID)
	if id == "" {
		return nil, errors.New("invalid --use-case-id: the Use Case id must be non-blank")
	}

	doc, err := decodeSpecObject(spec)
	if err != nil {
		return nil, err
	}

	if _, ok := doc["useCaseId"]; ok {
		return nil, errors.New(
			"spec already sets useCaseId; remove it from the spec or drop --use-case-id")
	}

	doc["useCaseId"] = id

	runtime := map[string]any{}

	if raw, ok := doc["runtime"]; ok && raw != nil {
		runtime, ok = raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid spec: 'runtime' must be an object")
		}
	}

	if _, ok := runtime["enclaveSelectionPolicy"]; !ok {
		runtime["enclaveSelectionPolicy"] = EnclaveSelectionPolicyAvailability
	}

	doc["runtime"] = runtime

	return json.Marshal(doc)
}
