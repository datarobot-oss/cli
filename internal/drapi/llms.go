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

package drapi

import (
	"github.com/datarobot/cli/internal/config"
)

type LLM struct {
	Model                string   `json:"model"`
	LlmID                string   `json:"llmId"`
	Name                 string   `json:"name"`
	Version              string   `json:"version"`
	Description          string   `json:"description"`
	Provider             string   `json:"provider"`
	Creator              string   `json:"creator"`
	ContextSize          int      `json:"contextSize"`
	MaxCompletionTokens  int      `json:"maxCompletionTokens"`
	Capabilities         []string `json:"capabilities"`
	SupportedLanguages   []string `json:"supportedLanguages"`
	InputTypes           []string `json:"inputTypes"`
	OutputTypes          []string `json:"outputTypes"`
	DocumentationLink    string   `json:"documentationLink"`
	DateAdded            string   `json:"dateAdded"`
	License              string   `json:"license"`
	IsPreview            bool     `json:"isPreview"`
	IsMetered            bool     `json:"isMetered"`
	RetirementDate       string   `json:"retirementDate"`
	SuggestedReplacement string   `json:"suggestedReplacement"`
	IsDeprecated         bool     `json:"isDeprecated"`
	IsActive             bool     `json:"isActive"`
	AvailableRegions     []string `json:"availableRegions"`

	ReferenceLinks []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"referenceLinks"`

	AvailableLitellmEndpoints struct {
		SupportsChatCompletions bool `json:"supportsChatCompletions"`
		SupportsResponses       bool `json:"supportsResponses"`
	} `json:"availableLitellmEndpoints"`
}

type LLMList struct {
	LLMs       []LLM  `json:"data"`
	Count      int    `json:"count"`
	TotalCount int    `json:"totalCount"`
	Next       string `json:"next"`
	Previous   string `json:"previous"`
}

func GetLLMs() (*LLMList, error) {
	url, err := config.GetEndpointURL("/api/v2/genai/llmgw/catalog/?limit=100")
	if err != nil {
		return nil, err
	}

	var llmList LLMList

	var active []LLM

	for url != "" {
		llmList = LLMList{}

		err = GetJSON(url, "LLMs", &llmList)
		if err != nil {
			return nil, err
		}

		for _, llm := range llmList.LLMs {
			if llm.IsActive {
				active = append(active, llm)
			}
		}

		url = llmList.Next
	}

	llmList.LLMs = active

	return &llmList, nil
}
