// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"strings"
)

func UpdatedDotenv(prompts []UserPrompt) string {
	var contents strings.Builder

	for _, prompt := range prompts {
		if prompt.SkipSaving() {
			continue
		}

		contents.WriteString(prompt.String())
		contents.WriteString("\n")
	}

	return contents.String()
}
