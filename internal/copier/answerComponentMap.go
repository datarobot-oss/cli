// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package copier

// TODO: I don't know what we should add here
type Details struct {
	ReadMeContents string
}

// Map the repo listed in an "answer file" to relevant info for component
// To Note: Not all of the README contents have been added
var ComponentDetailsMap = map[string]Details{
	"git@github.com:datarobot/af-component-agent.git": {
		ReadMeContents: `
<p align="center">
  <a href="https://github.com/datarobot-community/datarobot-agent-templates">
    <img src="docs/img/datarobot_logo.avif" width="600px" alt="DataRobot Logo"/>
  </a>
</p>
<h3 align="center">DataRobot Application Framework</h3>
<h1 align="center">af-component-agent</h1>

<p align="center">
  <a href="https://datarobot.com">Homepage</a>
  ·
  <a href="https://docs.datarobot.com/en/docs/agentic-ai/agentic-develop/index.html">Agent Documentation</a>
  ·
  <a href="https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html">Support</a>
</p>

<p align="center">
  <a href="https://github.com/datarobot/datarobot-agent-templates/tags">
    <img src="https://img.shields.io/github/v/tag/datarobot/af-component-agent?label=version" alt="Latest Release">
  </a>
  <a href="/LICENSE">
    <img src="https://img.shields.io/github/license/datarobot/af-component-agent" alt="License">
  </a>
</p>

The agent template provides a set of utilities for constructing a single or multi-agent workflow using frameworks such
as Nvidia NAT, CrewAI, LangGraph, LlamaIndex, and others. The template is designed to be flexible and extensible, allowing you
to create a wide range of agent-based applications.

The Agent Framework is component from the [DataRobot App Framework Studio](https://github.com/datarobot/app-framework-studio)


## Getting Started

To use this template, it expects the base component https://github.com/datarobot/af-component-base has already been
installed. To do that first, run:
` + "```" + `bash
uvx copier copy https://github.com/datarobot/af-component-base .
# uvx copier copy git@github.com:datarobot/af-component-base.git .
` + "```" + `

To add the agent component to your project, you can use the ` + "`" + `uvx copier` + "`" + ` command to copy the template from this repository:
` + "```" + `bash
uvx copier copy https://github.com/datarobot/af-component-agent .
# uvx copier copy git@github.com:datarobot/af-component-agent.git .
` + "```" + `

To update an existing agent template, you can use the ` + "`" + `uvx copier update` + "`" + ` command. This will update the template files
` + "```" + `bash
uvx copier update -a .datarobot/answers/agent-{ agent_app }.yml -A
` + "```" + `


## Developer Guide
Please see the [Development Documentation](/docs/development.md).


# Get help

If you encounter issues or have questions, try the following:

- Check [the documentation](#available-templates) for your chosen framework.
- [Contact DataRobot](https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html) for support.
- Open an issue on the [GitHub repository](https://github.com/datarobot/af-component-llm).`,
	},
	"git@github.com:datarobot/af-component-base.git": {
		ReadMeContents: `
# af-component-base

The base template for [App Framework Studio](https://github.com/datarobot/app-framework-studio)

Covers the basic structure and answers needed for a composition of App Templates

* Part of https://datarobot.atlassian.net/wiki/spaces/BOPS/pages/6542032899/App+Framework+-+Studio


## Instructions

To start for a repo:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-base .` + "`" + `


To update

` + "`" + `uvx copier update -a .datarobot/answers/base.yml -A` + "`",
	},
	"git@github.com:datarobot/af-component-fastapi-backend.git": {
		ReadMeContents: `
# af-component-fastapi-backend

The FastAPI Backned One-to-Many component from [App Framework Studio](https://github.com/datarobot/app-framework-studio)

Covers the basic structure and answers needed to have a basic FastAPI
app that is deployable as part of an App Template and can serve a
React Frontend Component:
https://github.com/datarobot/af-component-react


* Part of https://datarobot.atlassian.net/wiki/spaces/BOPS/pages/6542032899/App+Framework+-+Studio


## Instructions

To start for a repo:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-fastapi-backend .` + "`" + `

If a template requires multiple FastAPI backends, it can be used multiple times with a different answer to the ` + "`" + `fastapi_app` + "`" + ` question.

To work, it expects the base component https://github.com/datarobot/af-component-base has already been installed. To do that first, run:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-base .` + "`" + `


To update

` + "`" + `uvx copier update -a .datarobot/answers/fastapi-{{ fastapi_app }}.yml -A` + "`" + `

To update all templates that are copied:

` + "`" + `uvx copier update -a .datarobot/answers/* -A` + "`" + `

or just

` + "`" + `uvx copier update -a .datarobot/*` + "`",
	},
	"git@github.com:datarobot/af-component-fastmcp-backend.git": {
		ReadMeContents: `
# af-component-fastmcp-backend

The FastMCP Backned One-to-Many component from [App Framework Studio](https://github.com/datarobot/app-framework-studio)

Covers the basic structure and answers needed to have a basic FastMCP
app that is deployable as part of an App Template and can serve a
React Frontend Component:
https://github.com/datarobot/af-component-react


* Part of https://datarobot.atlassian.net/wiki/spaces/BOPS/pages/6542032899/App+Framework+-+Studio


## Instructions

To start for a repo:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-fastmcp-backend .` + "`" + `

If a template requires multiple FastMCP backends, it can be used multiple times with a different answer to the ` + "`" + `fastmcp_app` + "`" + ` question.

To work, it expects the base component https://github.com/datarobot/af-component-base has already been installed. To do that first, run:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-base .` + "`" + `


To update

` + "`" + `uvx copier update -a .datarobot/answers/fastmcp-{{ fastmcp_app }}.yml -A` + "`" + `

To update all templates that are copied:

` + "`" + `uvx copier update -a .datarobot/answers/* -A` + "`" + `

or just

` + "`" + `uvx copier update -a .datarobot/*` + "`",
	},
	"git@github.com:datarobot/af-component-llm.git": {
		ReadMeContents: `
<h3 align="center">DataRobot Application Framework</h3>
<h1 align="center">af-component-llm</h1>

<p align="center">
  <a href="https://datarobot.com">Homepage</a>
  ·
  <a href="https://docs.datarobot.com/en/docs/agentic-ai/agentic-develop/index.html">Agent Documentation</a>
  ·
  <a href="https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html">Support</a>
</p>

<p align="center">
  <a href="https://github.com/datarobot/af-component-llm/tags">
    <img src="https://img.shields.io/github/v/tag/datarobot/af-component-llm?label=version" alt="Latest Release">
  </a>
  <a href="/LICENSE">
    <img src="https://img.shields.io/github/license/datarobot/af-component-llm" alt="License">
  </a>
</p>

The LLM component provides the Buzok components required for configuring and using the LLM gateway or other LLM choices
such as an already deployed model.

The LLM is a component from the [DataRobot App Framework Studio](https://github.com/datarobot/app-framework-studio)


## Getting Started

To use this template, it expects the base component https://github.com/datarobot/af-component-base has already been
installed. To do that first, run:
` + "```" + `bash
uvx copier copy https://github.com/datarobot/af-component-base .
# uvx copier copy git@github.com:datarobot/af-component-base.git .
` + "```" + `

To add the llm component to your project, you can use the ` + "`" + `uvx copier` + "`" + ` command to copy the template from this repository:
` + "```" + `bash
uvx copier copy https://github.com/datarobot/af-component-llm .
# uvx copier copy git@github.com:datarobot/af-component-llm.git .
` + "```" + `

To update an existing llm template, you can use the ` + "`" + `uvx copier update` + "`" + ` command. This will update the template files
` + "```" + `bash
uvx copier update -a .datarobot/answers/llm-{ llm_name }.yml -A
` + "```" + `


# Get help

If you encounter issues or have questions, try the following:

- [Contact DataRobot](https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html) for support.
- Open an issue on the [GitHub repository](https://github.com/datarobot/af-component-llm).`,
	},
	"git@github.com:datarobot/af-component-react.git": {
		ReadMeContents: `
# af-component-react

The React Frontend One-to-Many component from [App Framework Studio](https://github.com/datarobot/app-framework-studio)

Covers the basic structure and answers needed to have a basic React app that is deployable as part of an App Template

* Part of https://datarobot.atlassian.net/wiki/spaces/BOPS/pages/6542032899/App+Framework+-+Studio


## Instructions

To start for a repo:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-react .` + "`" + `

If a template requires multiple React frontends, it can be used multiple times with a different answer to the ` + "`" + `react_app` + "`" + ` question.

To work, it expects the base component https://github.com/datarobot/af-component-base has already been installed. To do that first, run:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-base .` + "`" + `

and it also needs a web host as the backend to the frontend:

` + "`" + `uvx copier copy https://github.com/datarobot/af-component-fastapi-backend .` + "`" + `


To update

` + "`" + `uvx copier update -a .datarobot/answers/react-{{ react_app }}.yml -A` + "`" + `

To update all templates that are copied:

` + "`" + `uvx copier update -a .datarobot/answers/*.yaml -A` + "`",
	},
}
