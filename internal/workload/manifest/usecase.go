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

package manifest

import (
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// enclavePolicyAvailability is the workload package's
// EnclaveSelectionPolicyAvailability, restated so this package stays
// standalone.
const enclavePolicyAvailability = "availability"

// UseCaseID returns the Use Case the manifest links the workload to, "" when
// it names none. The key is the create endpoint's own useCaseId, so it
// reaches the create payload unchanged.
func (m *Manifest) UseCaseID() string {
	return topLevelString(m.root, keyUseCaseID)
}

// SetUseCase writes useCaseId into rendered manifest content, after
// importance. With onEnclave it also sets runtime.enclaveSelectionPolicy to
// availability unless the runtime already names a policy: the create endpoint
// refuses a Use Case that has Enclaves without one, and availability is the
// policy that needs no Enclave chosen by name.
func SetUseCase(content []byte, id string, onEnclave bool) ([]byte, error) {
	var doc yaml.Node

	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse the manifest to record the Use Case: %w", err)
	}

	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("cannot record the Use Case: root must be a YAML mapping")
	}

	root := doc.Content[0]

	if err := checkAliases(root); err != nil {
		return nil, fmt.Errorf("cannot record the Use Case: %w", err)
	}

	setTopLevel(root, keyUseCaseID, id, keyImportance, keyName)

	if onEnclave {
		runtime := mapValue(root, keyRuntime)
		if runtime == nil || runtime.Kind != yaml.MappingNode {
			runtime = mapping()
			root.Content = append(root.Content, scalar(keyRuntime), runtime)
		}

		if isNullish(mapValue(runtime, keyEnclavePolicy)) {
			setTopLevel(runtime, keyEnclavePolicy, enclavePolicyAvailability)
		}
	}

	rendered, err := encodeDoc(&doc)
	if err != nil {
		return nil, fmt.Errorf("cannot render the manifest: %w", err)
	}

	return spaceTopLevelBlocks(rendered), nil
}

// setTopLevel sets key in a mapping: in place when it is there, otherwise
// right after the first of after that is present, otherwise at the end.
func setTopLevel(node *yaml.Node, key, value string, after ...string) {
	if i := keyIndex(node, key); i >= 0 {
		node.Content[i+1] = scalar(value)

		return
	}

	at := len(node.Content)

	for _, anchor := range after {
		if i := keyIndex(node, anchor); i >= 0 {
			at = i + 2

			break
		}
	}

	node.Content = slices.Insert(node.Content, at, scalar(key), scalar(value))
}

func keyIndex(node *yaml.Node, key string) int {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}

	return -1
}
