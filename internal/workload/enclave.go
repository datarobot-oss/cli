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

	"github.com/datarobot/cli/internal/usecase"
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

// SpecSetsUseCase reports whether a JSON workload create spec already names
// a Use Case: a non-blank string in its top-level useCaseId field. A missing,
// null or blank useCaseId names none, and neither does a value of another
// type, which ApplyUseCase refuses. A spec that does not decode is reported as
// not setting one; validation rejects it later with a clearer error.
func SpecSetsUseCase(spec []byte) bool {
	doc, err := decodeSpecObject(spec)
	if err != nil {
		return false
	}

	named, err := specUseCase(doc)

	return err == nil && named
}

// specUseCase applies the one rule both SpecSetsUseCase and ApplyUseCase use:
// useCaseId names a Use Case only when it is a non-blank string. Absent, null
// and blank values name none; any other type is an invalid spec.
func specUseCase(doc map[string]any) (bool, error) {
	switch value := doc["useCaseId"].(type) {
	case nil:
		return false, nil
	case string:
		return strings.TrimSpace(value) != "", nil
	default:
		return false, errors.New("invalid spec: 'useCaseId' must be a string")
	}
}

// ApplyUseCase links a workload create spec to a Use Case: the top-level
// useCaseId field the server reads at create time. It leaves the placement
// alone: a Use Case on its own is an organizational link, and the workload
// goes to an Enclave only when the spec (or --enclave) sets an
// enclaveSelectionPolicy. It errors if the spec already names a Use Case
// rather than silently rewriting it; a null or blank useCaseId names none and
// is filled in. Re-encodes the spec the same way ApplyEnclavePin does.
func ApplyUseCase(spec []byte, id usecase.ID) (json.RawMessage, error) {
	doc, err := decodeSpecObject(spec)
	if err != nil {
		return nil, err
	}

	named, err := specUseCase(doc)
	if err != nil {
		return nil, err
	}

	if named {
		return nil, errors.New(
			"spec already sets useCaseId; remove it from the spec or drop --use-case-id")
	}

	doc["useCaseId"] = string(id)

	return json.Marshal(doc)
}
