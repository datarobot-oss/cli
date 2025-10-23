// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package regexp2

import (
	"regexp"
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
