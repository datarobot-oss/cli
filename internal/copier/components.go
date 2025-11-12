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
}

//go:embed readme/*.html
var readmeFS embed.FS

func init() {
	for key, details := range ComponentDetailsMap {
		contents, err := readmeFS.ReadFile("readme/" + details.readMeFile)
		if err == nil {
			details.ReadMeContents = string(contents)
			ComponentDetailsMap[key] = details
		}
	}
}

// Map the repo listed in an "answer file" to relevant info for component
// To Note: Not all of the README contents have been added
var ComponentDetailsMap = map[string]Details{
	"git@github.com:datarobot/af-component-agent.git": {
		readMeFile: "af-component-agent.html",
	},
	"git@github.com:datarobot/af-component-base.git": {
		readMeFile: "af-component-base.html",
	},
	"git@github.com:datarobot/af-component-fastapi-backend.git": {
		readMeFile: "af-component-fastapi-backend.html",
	},
	"git@github.com:datarobot/af-component-fastmcp-backend.git": {
		readMeFile: "af-component-fastmcp-backend.html",
	},
	"git@github.com:datarobot/af-component-llm.git": {
		readMeFile: "af-component-llm.html",
	},
	"git@github.com:datarobot/af-component-react.git": {
		readMeFile: "af-component-react.html",
	},
}
