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
	// pyenv strategy first — if the user manages Python via pyenv, pip install
	// is the most natural path; brew/asdf/curl follow in priority order.
	ToolRegistry["uv"] = ToolInfo{
		Name: "uv",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "pyenv", Commands: []string{"pip install uv"}},
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install uv"}},
			ManagerStrategy{Manager: "asdf", Commands: []string{
				"asdf plugin add uv https://github.com/asdf-community/asdf-uv.git",
				"asdf install uv latest",
				"asdf global uv latest",
			}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install astral-sh.uv"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install uv"}},
			FallbackStrategy{
				Commands:        []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"},
				CommandsWindows: []string{`powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`},
				URL:             "https://docs.astral.sh/uv/getting-started/installation/",
			},
		},
	}
}
