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

package shared

import (
	"fmt"
	"strings"
)

// ParseDataArgs parses --data arguments in key=value format
func ParseDataArgs(dataArgs []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, arg := range dataArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --data format: %s (expected key=value)", arg)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("empty key in --data argument: %s", arg)
		}

		// Try to parse boolean values
		if value == "true" {
			result[key] = true
			continue
		}

		if value == "false" {
			result[key] = false
			continue
		}

		// Otherwise store as string
		result[key] = value
	}

	return result, nil
}
