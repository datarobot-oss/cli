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

package registry

func init() {
	ToolRegistry["node"] = ToolInfo{
		Name:    "Node.js",
		Aliases: []string{"node.js", "nodejs"},
		Strategies: []Strategy{
			ManagerStrategy{Manager: "nvm", DefaultVersion: "24", Commands: []string{"nvm install {version}", "nvm use {version}"}},
			ManagerStrategy{Manager: "fnm", DefaultVersion: "24", Commands: []string{"fnm install {version}", "fnm use {version}"}},
			ManagerStrategy{Manager: "asdf", DefaultVersion: "24", Commands: []string{"asdf install nodejs {version}", "asdf global nodejs {version}"}},
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install node"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install OpenJS.NodeJS"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install nodejs"}},
			FallbackStrategy{
				DefaultVersion: "24",
				Message:        "Install a version manager (recommended):",
				Commands: []string{
					"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash",
					"# Restart terminal, then:",
					"nvm install {version}",
					"nvm use {version}",
				},
				URL: "https://nodejs.org/",
			},
		},
	}
}
