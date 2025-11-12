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
```bash
uvx copier copy https://github.com/datarobot/af-component-base .
# uvx copier copy git@github.com:datarobot/af-component-base.git .
```

To add the agent component to your project, you can use the `uvx copier` command to copy the template from this repository:
```bash
uvx copier copy https://github.com/datarobot/af-component-agent .
# uvx copier copy git@github.com:datarobot/af-component-agent.git .
```

To update an existing agent template, you can use the `uvx copier update` command. This will update the template files
```bash
uvx copier update -a .datarobot/answers/agent-{ agent_app }.yml -A
```


## Developer Guide
Please see the [Development Documentation](/docs/development.md).


# Get help

If you encounter issues or have questions, try the following:

- Check [the documentation](#available-templates) for your chosen framework.
- [Contact DataRobot](https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html) for support.
- Open an issue on the [GitHub repository](https://github.com/datarobot/af-component-llm).
