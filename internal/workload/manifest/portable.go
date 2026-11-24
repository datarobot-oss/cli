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

// Portable returns the live view for another install: no workload binding, and
// every credential reference reset to CredentialPlaceholder, since both are
// issued per install. The second value is how many references were reset.
func (l Live) Portable() (Live, int) {
	portable := l
	portable.WorkloadID = ""
	portable.Spec = deepCopyMap(l.Spec)
	portable.Runtime = deepCopyMap(l.Runtime)

	return portable, resetCredentialIDs(portable.Spec) + resetCredentialIDs(portable.Runtime)
}

// resetCredentialIDs resets every credential-backed environment variable under node.
func resetCredentialIDs(node any) int {
	reset := 0

	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == keyEnvironmentVars {
				reset += resetCredentialEntries(value)

				continue
			}

			reset += resetCredentialIDs(value)
		}
	case []any:
		for _, item := range typed {
			reset += resetCredentialIDs(item)
		}
	}

	return reset
}

func resetCredentialEntries(vars any) int {
	entries, _ := vars.([]any)
	reset := 0

	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok || stringAt(entry, keySource) != credentialSource {
			continue
		}

		entry[keyDRCredentialID] = CredentialPlaceholder
		reset++
	}

	return reset
}
