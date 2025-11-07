// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"regexp"

	"github.com/datarobot/cli/internal/misc/regexp2"
)

// Variable represents a parsed environment variable from a .env file or template.
type Variable struct {
	Name      string
	Value     string
	Commented bool
}

// ParseVariables parses variables from template lines without attempting to auto-populate them.
// This is used when parsing .env files to extract variable names and values.
// Commented lines (starting with #) are marked as such.
func ParseVariables(templateLines []string) []Variable {
	variables := make([]Variable, 0)

	for _, templateLine := range templateLines {
		v := parseVariableFromLine(templateLine)

		if v.Name != "" {
			variables = append(variables, v)
		}
	}

	return variables
}

func parseVariableFromLine(line string) Variable {
	expr := regexp.MustCompile(`^(?P<commented>\s*#\s*)?(?P<name>[a-zA-Z_]+[a-zA-Z0-9_]*) *= *(?P<value>[^\n]*)\n$`)
	result := regexp2.NamedStringMatches(expr, line)
	v := Variable{}

	v.Name = result["name"]
	v.Value = result["value"]

	if result["commented"] != "" {
		v.Commented = true
	}

	return v
}
