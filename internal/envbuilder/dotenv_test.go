// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import "testing"

func TestDotenvFromPromptsMerged(t *testing.T) {
	prompts := []UserPrompt{
		{
			Active: true,
			Value:  "env value updated",
			Env:    "ENV",
			Key:    "",
			Help:   "env help",
		},
		{
			Active: true,
			Value:  "key value updated",
			Env:    "",
			Key:    "key",
			Help:   "key help",
		},
	}

	contents := `# initial comment
# extra comment 1
extra1=value1
ENV="env value"
# extra comment 2
extra2=value2
# extra comment 3
extra3=value3
# key="key value"
# extra comment 4
extra4=value4
`

	contentsExpected := `# initial comment
# extra comment 1
extra1=value1

# env help
ENV="env value updated"
# extra comment 2
extra2=value2
# extra comment 3
extra3=value3

# key help
# key="key value updated"
# extra comment 4
extra4=value4
`

	result := mergedDotenvChunks(prompts, contents)

	resultString := result.String()

	if resultString != contentsExpected {
		t.Errorf("got:\n=================\n%s=================\n", resultString)
	}
}
