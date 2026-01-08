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

package copier

import (
	"embed"
)

// TODO: I don't know what we should add here
type Details struct {
	readMeFile     string
	ReadMeContents string

	Name      string
	ShortName string
	RepoURL   string
	Enabled   bool
}

//go:embed readme/*.md
var readmeFS embed.FS

func init() {
	for i, details := range ComponentDetails {
		contents, err := readmeFS.ReadFile("readme/" + details.readMeFile)
		if err == nil {
			ComponentDetails[i].ReadMeContents = string(contents)
		}
	}

	for _, details := range ComponentDetails {
		ComponentDetailsByURL[details.RepoURL] = details
		ComponentDetailsByShortName[details.ShortName] = details

		if details.Enabled {
			EnabledComponents = append(EnabledComponents, details)
			EnabledShortNames = append(EnabledShortNames, details.ShortName)
		}
	}
}

// Map the repo listed in an "answer file" to relevant info for component
// To Note: Not all of the README contents have been added
var (
	ComponentDetailsByURL       = map[string]Details{}
	ComponentDetailsByShortName = map[string]Details{}
	EnabledComponents           = make([]Details, 0, len(ComponentDetails))
	EnabledShortNames           = make([]string, 0, len(ComponentDetails))
)

var ComponentDetails = []Details{
	{
		readMeFile: "af-component-agent.md",

		Name:      "Agent",
		ShortName: "agent",
		RepoURL:   "https://github.com/datarobot-community/af-component-agent.git",
		Enabled:   true,
	},
	{
		readMeFile: "af-component-base.md",

		Name:      "Base",
		ShortName: "base",
		RepoURL:   "https://github.com/datarobot/af-component-base.git",
	},
	{
		readMeFile: "af-component-fastapi-backend.md",

		Name:      "FastAPI backend",
		ShortName: "fastapi",
		RepoURL:   "https://github.com/datarobot/af-component-fastapi-backend.git",
	},
	{
		readMeFile: "af-component-fastmcp-backend.md",

		Name:      "FastMCP backend",
		ShortName: "fastmcp",
		RepoURL:   "https://github.com/datarobot/af-component-fastmcp-backend.git",
	},
	{
		readMeFile: "af-component-llm.md",

		Name:      "LLM",
		ShortName: "llm",
		RepoURL:   "https://github.com/datarobot/af-component-llm.git",
	},
	{
		readMeFile: "af-component-react.md",

		Name:      "React",
		ShortName: "react",
		RepoURL:   "https://github.com/datarobot/af-component-react.git",
	},
}
