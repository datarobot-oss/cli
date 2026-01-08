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
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`# extra comment 2`,
				`extra2=value2`,
				`# extra comment 3`,
				`extra3=value3`,
				`#`,
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
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`#`,
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
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`#`,
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
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`#`,
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
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`#`,
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
				`#`,
				`# key help`,
				`# key="key value updated"`,
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
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
				``,
			}, "\n"),
			expected: strings.Join([]string{
				`extra1=value1`,
				`extra2=value2`,
				`extra3=value3`,
				`#`,
				`# ENV help`,
				`ENV="env value updated"`,
				`#`,
				`# key help`,
				`# key="key value updated"`,
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

func TestMergedDotenvChunksSort(t *testing.T) {
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

	contents := strings.Join([]string{
		`extra1=value1`,
		`extra2=value2`,
		``,
	}, "\n")

	result := mergedDotenvChunks(prompts, contents)

	if result[0].PromptIndex != 0 {
		t.Errorf("result[0] should be user chunk")
	}

	if result[1].PromptIndex != 0 {
		t.Errorf("result[1] should be user chunk")
	}

	if result[2].PromptIndex == 0 {
		t.Errorf("result[2] should be prompt chunk")
	}

	if result[3].PromptIndex == 0 {
		t.Errorf("result[3] should be prompt chunk")
	}

	result.Sort()

	if result[0].PromptIndex != 0 {
		t.Errorf("sorted result[0] should be user chunk")
	}

	if result[1].PromptIndex != 0 {
		t.Errorf("sorted result[1] should be user chunk")
	}

	if result[2].PromptIndex == 0 {
		t.Errorf("sorted result[2] should be prompt chunk")
	}

	if result[3].PromptIndex == 0 {
		t.Errorf("sorted result[3] should be prompt chunk")
	}
}
