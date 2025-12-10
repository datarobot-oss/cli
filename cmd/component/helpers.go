// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"fmt"
	"strings"
)

// parseDataArgs parses --data arguments in key=value format
func parseDataArgs(dataArgs []string) (map[string]interface{}, error) {
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
