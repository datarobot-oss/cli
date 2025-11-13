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
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/spf13/viper"
)

// Variable represents a parsed environment variable from a .env file or template.
type Variable struct {
	Name        string
	Value       string
	Description string
	Secret      bool
	Commented   bool
}

type Variables []Variable

func (v *Variable) String() string {
	if v.Commented {
		return "# " + v.Name + "=" + v.Value + "\n"
	}

	return v.Name + "=" + v.Value + "\n"
}

// ParseVariablesOnly parses variables from template lines without attempting to auto-populate them.
// This is used when parsing .env files to extract variable names and values.
// Commented lines (starting with #) are marked as such.
func ParseVariablesOnly(templateLines []string) []Variable {
	variables := make([]Variable, 0)

	for _, templateLine := range templateLines {
		v := NewFromLine(templateLine)

		if v.Name != "" {
			variables = append(variables, v)
		}
	}

	return variables
}

func NewFromLine(line string) Variable {
	expr := regexp.MustCompile(`^(?P<commented>\s*#\s*)?(?P<name>[a-zA-Z_]+[a-zA-Z0-9_]*) *= *(?P<value>[^\n]*)\n$`)
	result := regexp2.NamedStringMatches(expr, line)

	return Variable{
		Name:      result["name"],
		Value:     result["value"],
		Secret:    knownVariables[result["name"]].secret,
		Commented: result["commented"] != "",
	}
}

func VariablesFromLines(lines []string) ([]Variable, string) {
	variables := make([]Variable, 0)

	var contents strings.Builder

	for _, line := range lines {
		v := NewFromLine(line)

		if v.Name != "" && v.Commented {
			variables = append(variables, v)
		}

		if v.Name == "" || v.Commented {
			contents.WriteString(line)
			continue
		}

		v.setValue()

		if v.Value == "" {
			contents.WriteString(line)
		} else {
			log.Info("Adding variable " + v.Name)
			contents.WriteString(v.String())
		}

		variables = append(variables, v)
	}

	return variables, contents.String()
}

func (v *Variable) setValue() {
	conf, found := knownVariables[v.Name]

	if !found {
		return
	}

	switch {
	case conf.viperKey != "":
		v.Value = viper.GetString(conf.viperKey)
	case conf.getValue != nil:
		var err error

		v.Value, err = conf.getValue()
		if err != nil && v.Value != "" {
			// Only log error if we actually got a non-empty value with an error
			// Ignore "empty url" and similar errors when exiting setup
			log.Error(err)
		}
	}
}
