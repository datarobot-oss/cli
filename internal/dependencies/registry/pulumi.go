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
	ToolRegistry["pulumi"] = ToolInfo{
		Name:    "Pulumi",
		Aliases: []string{"pulumi infrastructure as code tool"},
		Strategies: []Strategy{
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install pulumi"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install Pulumi.Pulumi"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install pulumi"}},
			FallbackStrategy{
				Commands:        []string{"curl -fsSL https://get.pulumi.com | sh"},
				CommandsWindows: []string{"iwr -useb https://get.pulumi.com/install.ps1 | iex"},
				URL:             "https://www.pulumi.com/docs/install/",
			},
		},
	}
}
