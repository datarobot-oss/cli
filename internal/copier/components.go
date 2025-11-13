// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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

		ComponentDetailsByURL[details.RepoURL] = details
		ComponentDetailsByShortName[details.ShortName] = details
	}
}

// Map the repo listed in an "answer file" to relevant info for component
// To Note: Not all of the README contents have been added
var (
	ComponentDetailsByURL       = map[string]Details{}
	ComponentDetailsByShortName = map[string]Details{}
)

var ComponentDetails = []Details{
	{
		readMeFile: "af-component-agent.md",

		Name:      "Agent",
		ShortName: "agent",
		RepoURL:   "git@github.com:datarobot/af-component-agent.git",
		Enabled:   true,
	},
	{
		readMeFile: "af-component-base.md",

		Name:      "Base",
		ShortName: "Base",
		RepoURL:   "git@github.com:datarobot/af-component-base.git",
	},
	{
		readMeFile: "af-component-fastapi-backend.md",

		Name:      "FastAPI backend",
		ShortName: "fastapi",
		RepoURL:   "git@github.com:datarobot/af-component-fastapi-backend.git",
	},
	{
		readMeFile: "af-component-fastmcp-backend.md",

		Name:      "FastMCP backend",
		ShortName: "fastmcp",
		RepoURL:   "git@github.com:datarobot/af-component-fastmcp-backend.git",
	},
	{
		readMeFile: "af-component-llm.md",

		Name:      "LLM",
		ShortName: "llm",
		RepoURL:   "git@github.com:datarobot/af-component-llm.git",
	},
	{
		readMeFile: "af-component-react.md",

		Name:      "React",
		ShortName: "react",
		RepoURL:   "git@github.com:datarobot/af-component-react.git",
	},
}
