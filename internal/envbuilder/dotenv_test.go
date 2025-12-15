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
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type TestCaseDotenvFromPromptsMerged struct {
	prompts  []UserPrompt
	contents string
	expected string
}

func TestDotenvFromPromptsMerged(t *testing.T) {
	prompts := []UserPrompt{
		{
			Active: true,
			Value:  "env value updated",
			Env:    "ENV",
			Key:    "",
			Help:   "ENV help",
		},
		{
			Active: true,
			Value:  "key value updated",
			Env:    "",
			Key:    "key",
			Help:   "key help",
		},
	}

	testCases := []TestCaseDotenvFromPromptsMerged{
		{
			prompts: prompts,
			contents: strings.Join([]string{
				`# extra comment 1`,
				`extra1=value1`,
				`ENV="env value old"`,
				`# extra comment 2`,
				`extra2=value2`,
				`# extra comment 3`,
				`extra3=value3`,
				`# key="key value old"`,
				`# extra comment 4`,
				`extra4=value4`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`# extra comment 1`,
				`extra1=value1`,
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				`# extra comment 2`,
				`extra2=value2`,
				`# extra comment 3`,
				`extra3=value3`,
				``,
				`# key help`,
				`# key="key value updated"`,
				`# extra comment 4`,
				`extra4=value4`,
				``,
			}, "\n"),
		}, {
			prompts: []UserPrompt{},
			contents: strings.Join([]string{
				`# extra comment 1`,
				`extra1=value1`,
				`ENV="env value old"`,
				`# extra comment 2`,
				`extra2=value2`,
				`# extra comment 3`,
				`extra3=value3`,
				`# key="key value old"`,
				`# extra comment 4`,
				`extra4=value4`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`# extra comment 1`,
				`extra1=value1`,
				`ENV="env value old"`,
				`# extra comment 2`,
				`extra2=value2`,
				`# extra comment 3`,
				`extra3=value3`,
				`# key="key value old"`,
				`# extra comment 4`,
				`extra4=value4`,
				``,
			}, "\n"),
		}, {
			prompts: prompts,
			contents: strings.Join([]string{
				`extra1=value1`,
				`ENV="env value old"`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`extra1=value1`,
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				``,
				`# key help`,
				`# key="key value updated"`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
		}, {
			prompts: prompts,
			contents: strings.Join([]string{
				`extra1=value1`,
				`extra2=value2`,
				`# key="key value old"`,
				`extra3=value3`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`extra1=value1`,
				`extra2=value2`,
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				``,
				`# key help`,
				`# key="key value updated"`,
				`extra3=value3`,
				``,
			}, "\n"),
		}, {
			prompts: prompts,
			contents: strings.Join([]string{
				`# key="key value old"`,
				`extra1=value1`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				``,
				`# key help`,
				`# key="key value updated"`,
				`extra1=value1`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
		}, {
			prompts: prompts,
			contents: strings.Join([]string{
				`extra1=value1`,
				`extra2=value2`,
				`extra3=value3`,
				`# key="key value old"`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`extra1=value1`,
				`extra2=value2`,
				`extra3=value3`,
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				``,
				`# key help`,
				`# key="key value updated"`,
				``,
			}, "\n"),
		}, {
			prompts: prompts,
			contents: strings.Join([]string{
				`extra1=value1`,
				`# key="key value old"`,
				`ENV="env value old"`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`extra1=value1`,
				``,
				`# key help`,
				`# key="key value updated"`,
				``,
				`# ENV help`,
				`ENV="env value updated"`,
				`extra2=value2`,
				`extra3=value3`,
				``,
			}, "\n"),
		},
	}

	for i, testCase := range testCases {
		result := DotenvFromPromptsMerged(testCase.prompts, testCase.contents)

		if result != testCase.expected {
			dmp := diffmatchpatch.New()
			diffs := dmp.DiffMain(testCase.expected, result, true)

			t.Errorf("testCase[%d] expected:\n=================\n%s=================\n", i, testCase.expected)
			t.Errorf("testCase[%d] got:\n=================\n%s=================\n", i, result)
			t.Errorf("testCase[%d] diff:\n=================\n%s=================\n", i, dmp.DiffPrettyText(diffs))
		}
	}
}
