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

// Enclave selection policies as serialized by the server. availability (the
// default) lets DataRobot place the workload; manual pins it to the Enclaves
// named in runtime.enclaves.
const (
	EnclaveSelectionPolicyAvailability = "availability"
	EnclaveSelectionPolicyManual       = "manual"
)

// ApplyEnclavePin pins a JSON workload create spec to a single Enclave:
// runtime.enclaveSelectionPolicy becomes "manual" and runtime.enclaves the
// one-element list the server accepts today. It errors if the spec already
// sets either field rather than silently rewriting it. Re-encodes the spec:
// numbers survive via json.Number, key order does not.
func ApplyEnclavePin(spec []byte, enclave string) ([]byte, error) {
	name := strings.TrimSpace(enclave)
	if name == "" {
		return nil, errors.New("invalid --enclave: the Enclave name must be non-blank")
	}

	dec := json.NewDecoder(bytes.NewReader(spec))
	dec.UseNumber()

	var doc map[string]any

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	// A top-level null decodes successfully into a nil map, and assigning
	// runtime into that would panic. Any other non-object (array, string)
	// already fails Decode above.
	if doc == nil {
		return nil, errors.New("invalid spec: the spec must be a JSON object")
	}

	runtime := map[string]any{}

	if raw, ok := doc["runtime"]; ok && raw != nil {
		runtime, ok = raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid spec: 'runtime' must be an object")
		}
	}

	// Presence alone is the conflict, not a differing value: a spec that says
	// enclaves: [] pinned nothing on purpose, and "the flag agrees with the
	// file" is not worth a second code path.
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
