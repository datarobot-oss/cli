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
	ToolRegistry["python"] = ToolInfo{
		Name: "Python",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "pyenv", DefaultVersion: "3.14", Commands: []string{"pyenv install {version}", "pyenv global {version}"}},
			ManagerStrategy{Manager: "asdf", DefaultVersion: "3.14", Commands: []string{"asdf install python {version}", "asdf global python {version}"}},
			ManagerStrategy{Manager: "brew", DefaultVersion: "3.14", Commands: []string{"brew install python@{version_mm}"}},
			ManagerStrategy{Manager: "winget", DefaultVersion: "3.14", Commands: []string{"winget install Python.Python.{version_mm}"}},
			ManagerStrategy{Manager: "choco", DefaultVersion: "3.14", Commands: []string{"choco install python --version={version}"}},
			FallbackStrategy{
				DefaultVersion: "3.14",
				Message:        "Install pyenv (recommended for managing Python versions):",
				Commands: []string{
					"curl https://pyenv.run | bash",
					"# Restart terminal, then:",
					"pyenv install {version}",
					"pyenv global {version}",
				},
				CommandsWindows: []string{
					"# Install pyenv-win via PowerShell:",
					`Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/pyenv-win/pyenv-win/master/pyenv-win/install-pyenv-win.ps1" -OutFile "./install-pyenv-win.ps1"; &"./install-pyenv-win.ps1"`,
					"# Restart terminal, then:",
					"pyenv install {version}",
					"pyenv global {version}",
				},
				URL: "https://www.python.org/downloads/",
			},
		},
	}
}
