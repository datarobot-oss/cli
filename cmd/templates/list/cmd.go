// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package list

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/spf13/cobra"
)

func Run() error {
	templateList, err := drapi.GetTemplates()
	if err != nil {
		return err
	}

	for _, template := range templateList.Templates {
		fmt.Printf("ID: %s\tName: %s\n", template.ID, template.Name)
	}

	return nil
}

var Cmd = &cobra.Command{
	Use:   "list",
	Short: "📋 List all available AI application templates",
	Long: `List all available AI application templates from DataRobot.

This command shows you all the pre-built templates you can use to quickly
start building AI applications. Each template includes:
  • Complete application structure
  • Pre-configured components
  • Documentation and examples
  • Ready-to-deploy setup

💡 Use 'dr templates setup' for an interactive selection experience.`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return auth.EnsureAuthenticatedE(cmd.Context())
	},
	Run: func(_ *cobra.Command, _ []string) {
		err := Run()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
