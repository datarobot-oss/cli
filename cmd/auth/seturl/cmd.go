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

package seturl

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-url [url]",
		Short: "🌐 Configure your DataRobot environment URL.",
		Long: `Configure your DataRobot environment URL with an interactive selection.

This command helps you choose the correct DataRobot environment:
  • US Cloud (most common): https://app.datarobot.com
  • EU Cloud: https://app.eu.datarobot.com
  • Japan Cloud: https://app.jp.datarobot.com
  • Custom/On-Premise: Your organization's DataRobot URL

💡 If you're unsure, check the URL you use to log in to DataRobot in your browser.`,
		Run: func(cmd *cobra.Command, args []string) {
			var url string
			if len(args) > 0 {
				url = args[0]
			}

			if url != "" {
				err := config.SaveURLToConfig(url)
				if err == nil {
					_ = auth.EnsureAuthenticatedE(cmd, args)

					return
				}
			}

			urlChanged := auth.SetURLAction()

			if urlChanged {
				_ = auth.EnsureAuthenticatedE(cmd, args)
			}
		},
	}
}
