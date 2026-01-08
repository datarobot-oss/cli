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

package regexp2

import (
	"regexp"
	"strconv"
)

func NamedStringMatches(expr *regexp.Regexp, str string) map[string]string {
	match := expr.FindStringSubmatch(str)
	result := make(map[string]string)
	matchLen := len(match)

	for i, name := range expr.SubexpNames() {
		if i > matchLen {
			break
		}

		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	return result
}

func NamedIntMatches(expr *regexp.Regexp, str string) map[string]int {
	if !expr.MatchString(str) {
		return nil
	}

	match := expr.FindStringSubmatch(str)
	result := make(map[string]int)
	matchLen := len(match)

	for i, name := range expr.SubexpNames() {
		if i > matchLen {
			break
		}

		if i != 0 && name != "" {
			result[name], _ = strconv.Atoi(match[i])
		}
	}

	return result
}
