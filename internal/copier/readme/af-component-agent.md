# Agent component template

The DataRobot agent component template provides a set of utilities for constructing a single or multi-agent workflow using frameworks such as Nvidia NAT, CrewAI, LangGraph, LlamaIndex, and others. The template is designed to be flexible and extensible, allowing you to create a wide range of agent-based applications.

This component enables you to build intelligent agent workflows that can handle complex tasks, coordinate multiple agents, and integrate with various AI frameworks. Whether you're building a single-agent application or a sophisticated multi-agent system, this template provides the foundation and utilities needed to get started quickly.

## Getting Started

To add the agent component to your project, run the following command to copy the template:

```bash
uvx copier copy https://github.com/datarobot-community/af-component-agent .
```

To update an existing agent template, run the following command to update the template files:

```bash
uvx copier update -a .datarobot/answers/agent-{{ agent_app }}.yml -A
```

## Developer Guide

Please see the [development documentation](https://github.com/datarobot-community/af-component-agent/blob/main/docs/development.md) for more information on how to develop the agent component template.

## Get help

If you encounter issues or have questions:

- [Contact DataRobot](https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html) for support.
- Open an issue on the [GitHub repository](https://github.com/datarobot-community/af-component-agent).
