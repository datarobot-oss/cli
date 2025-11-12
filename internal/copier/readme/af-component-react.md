# af-component-react

The React Frontend One-to-Many component from [App Framework Studio](https://github.com/datarobot/app-framework-studio)

Covers the basic structure and answers needed to have a basic React app that is deployable as part of an App Template

* Part of https://datarobot.atlassian.net/wiki/spaces/BOPS/pages/6542032899/App+Framework+-+Studio


## Instructions

To start for a repo:

`uvx copier copy https://github.com/datarobot/af-component-react .`

If a template requires multiple React frontends, it can be used multiple times with a different answer to the `react_app` question.

To work, it expects the base component https://github.com/datarobot/af-component-base has already been installed. To do that first, run:

`uvx copier copy https://github.com/datarobot/af-component-base .`

and it also needs a web host as the backend to the frontend:

`uvx copier copy https://github.com/datarobot/af-component-fastapi-backend .`


To update

`uvx copier update -a .datarobot/answers/react-{{ react_app }}.yml -A`

To update all templates that are copied:

`uvx copier update -a .datarobot/answers/*.yaml -A`
