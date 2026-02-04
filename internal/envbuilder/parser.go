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

package envbuilder

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/joho/godotenv"
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
	quotedValue := strconv.Quote(v.Value)

	if v.Commented {
		return "# " + v.Name + "=" + quotedValue + "\n"
	}

	return v.Name + "=" + quotedValue + "\n"
}

func (v *Variable) StringSecret() string {
	if v.Secret {
		secretLength := len(v.Value)
		if secretLength > 20 {
			secretLength = 20
		}

		secret := strings.Repeat("*", secretLength)

		return "# " + v.Name + "=" + secret + "\n"
	}

	return v.String()
}

// ParseVariablesOnly parses variables from lines without attempting to auto-populate them.
// This is used when parsing .env files to extract variable names and values.
// Commented lines (starting with #) are marked as such.
func ParseVariablesOnly(dotenvLines []string) []Variable {
	unquotedValues, _ := godotenv.Unmarshal(strings.Join(dotenvLines, "\n"))
	variables := make([]Variable, 0)

	for _, templateLine := range dotenvLines {
		v := NewFromLine(templateLine, unquotedValues)

		if v.Name != "" {
			variables = append(variables, v)
		}
	}

	return variables
}

func NewFromLine(line string, unquotedValues map[string]string) Variable {
	expr := regexp.MustCompile(`^(?P<commented>\s*#\s*)?(?P<name>[a-zA-Z_]+[a-zA-Z0-9_]*) *= *(?P<value>[^\n]*)\n$`)
	result := regexp2.NamedStringMatches(expr, line)

	if unquotedValue, ok := unquotedValues[result["name"]]; ok {
		result["value"] = unquotedValue
	} else if unquotedResultValue, err := strconv.Unquote(result["value"]); err == nil {
		result["value"] = unquotedResultValue
	}

	return Variable{
		Name:      result["name"],
		Value:     result["value"],
		Commented: result["commented"] != "",
	}
}

func VariablesFromLines(lines []string) ([]Variable, string) {
	unquotedValues, _ := godotenv.Unmarshal(strings.Join(lines, "\n"))
	variablesConfig := knownVariables(unquotedValues)

	variables := make([]Variable, 0)

	var contents strings.Builder

	for _, line := range lines {
		v := NewFromLine(line, unquotedValues)

		if v.Name != "" && v.Commented {
			variables = append(variables, v)

			contents.WriteString(line)

			continue
		}

		if v.Name == "" || v.Commented {
			contents.WriteString(line)
			continue
		}

		if varConfig, ok := variablesConfig[v.Name]; ok {
			v.Value = varConfig.value
			v.Secret = varConfig.secret
		}

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
