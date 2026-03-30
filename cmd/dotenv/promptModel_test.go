// Copyright 2026 DataRobot, Inc. and its affiliates.
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

package dotenv

import (
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/stretchr/testify/assert"
)

func TestLLMsToPromptOptions(t *testing.T) {
	llms := []drapi.LLM{
		{LlmID: "1", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true},
		{LlmID: "2", Name: "Claude 3", Provider: "anthropic", Model: "claude-3-sonnet", IsActive: true},
	}

	options := llmsToPromptOptions(llms)

	assert.Len(t, options, 2)

	assert.Equal(t, envbuilder.PromptOption{
		Blank:    false,
		Checked:  false,
		Name:     "GPT-4o (azure)",
		Value:    "datarobot/gpt-4o",
		Requires: "",
	}, options[0])

	assert.Equal(t, envbuilder.PromptOption{
		Blank:    false,
		Checked:  false,
		Name:     "Claude 3 (anthropic)",
		Value:    "datarobot/claude-3-sonnet",
		Requires: "",
	}, options[1])
}

func TestLLMsToPromptOptions_Empty(t *testing.T) {
	options := llmsToPromptOptions([]drapi.LLM{})

	assert.Empty(t, options)
}
