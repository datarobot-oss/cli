# BugBot Review Instructions

You are a code review bot. When reviewing pull requests in this repository, use the guideline below to identify issues and leave comments. The guideline defines critical areas that require sign-offs, general rules for public repositories, changelog expectations, and file operation best practices.

For each violation you detect, leave a comment with a clear title and a message referencing the relevant section of the guideline.

Full guideline source: https://datarobot.atlassian.net/wiki/spaces/BUZOK/pages/7305920528/REVIEW+BEFORE+COMMIT+Working+with+agentic+starter+application+and+its+components

---

# REVIEW BEFORE COMMIT: Working with agentic starter application and its components

## Background

Changes to datarobot/recipe-datarobot-agent-application proves to be high-impact to our customers, and because of the dynamic nature of the industry we do not make all changes under reviewed PBMPs with PRODUCT and DOC sign-offs: we have to pro-actively add improvements based on feedback. Therefore we need to know which changes are necessary to be reviewed/communicated with stakeholders.

This document outlines common guidelines which should be applied when making change or reviewing PRs in datarobot/recipe-datarobot-agent-application and related components:

- datarobot-community/af-component-agent
- datarobot-community/af-component-datarobot-mcp
- datarobot-oss/datarobot-genai
- datarobot-oss/cli

### Related

- Application Framework Design Principles
- App Framework Overview and Architecture
- Template Customization and Maintenance Strategy

## Stakeholders

| Stakeholder | Person | Approval |
|---|---|---|
| PRODUCT | @Nate Daly | |
| TECHLEAD | @Anatolii Stehnii | |
| DOC | @Shawn Magee @Jen Davies | |
| APPS-Customer Engineering | @Carson Gee @Luke Shulman @Zach Ingbretsen | |

## A. Critical areas

### A1. Renaming and wording

Requires: PRODUCT sign-off, DOC ticket

Template names, component names, primary module names (myagent.py) are considered highly important properties of UX. Their names are approved by PRODUCT, and any changes should be reflected in documentation.

### A2. Backward compatibility

Requires: TECHLEAD sign-off, PRODUCT sign-off, DOC ticket

See Backward compatibility of agentic application templates for more context.

### A10. Changes to dr start

Requires: PRODUCT and @Carson Gee sign-off, DOC ticket

dr start is the key part of UX of agentic starter application, and any changes to it have to be reviewed by PRODUCT ( @Nate Daly ). This includes direct changes to task start, and indirect changes:

- new configuration options in agent or mcp (see Message from Anatolii Stehnii in #agentic-flow-dev for example)
- Change to dr start logic in CLI itself

Tired waiting review from @Carson Gee ? Try splitting your PR, and deliver critical changes separately!

We SHOULD BE mindful about the number of questions we are asking from users in components configuration, and try reducing them as much as possible. Related initiatives:

- Component discovery: Message from Anatolii Stehnii in #dr-cli
- Do not ask optional question: TBD

### A11. Adding or removing tasks in all components

Requires: @Carson Gee sign-off, DOC review

task is essentially our TUI, and we should be mindful adding or removing them as this may be just as disruptive as adding or removing GUI elements.

### A15. Adding or removing tools, prompts, resources to MCP

Requires: PRODUCT sign-off, DOC review

MCP is a key element of an agent which defines what agent can or can not do. Adding new tools (or other resources) without restrain will lead to poor experience, hence only vetted (read: most important) tools should be added to the default set available OOTB.

### A20. Changes to agent interface

Requires: PRODUCT review, DOC ticket

myagent.py is a core part of an agent definition, and changing it means users will have to migrate their agents if they want to merge upstream. This also means our documentation have to be updated to reflect a new interface for users.

## B. General guidelines

### B1. Assume public

Repositories linked at the top are either public, or meant to become public. Therefore:

- Do not include internal code, proprietary logic, or private repository references.
- Avoid leaking internal architecture, infrastructure details, or security mechanisms.
- Keep comments, README and any other communication language civil, polite, and free of internal references

### B2. Update CHANGELOG

Use past tense

When CHANGELOG.md is present, and your change looks like it should be communicated with users, please add it.

Do not add every change: Added .woff2 and .woff and .js files to fastapi_server/static/.gitignore is not important to our users.

When bumping child components, list all the changes:

```
- Updated agent component from 11.6.3 to 11.6.10:
  - Migrated to a new interface
  - Refactored agent infra concurrency configuration
  - Fixed header forwarding in LangGraph
  - Added debugpy for debugging in IDE
```

### B10. Use git rename when moving files

copier relies on git history when it updates component and merges with local changes. Recently we have moved agent.py → myagent.py, and this broke downstream templates, so we had to add a custom migration.
