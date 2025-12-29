// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package seturl

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-url",
		Short: "🌐 Configure your DataRobot environment URL.",
		Long: `Configure your DataRobot environment URL with an interactive selection.

This command helps you choose the correct DataRobot environment:
  • US Cloud (most common): https://app.datarobot.com
  • EU Cloud: https://app.eu.datarobot.com
  • Japan Cloud: https://app.jp.datarobot.com
  • Custom/On-Premise: Your organization's DataRobot URL

💡 If you're unsure, check the URL you use to log in to DataRobot in your browser.`,
		Run: func(cmd *cobra.Command, _ []string) {
			urlChanged := auth.SetURLAction()

			if urlChanged {
				auth.EnsureAuthenticated(cmd.Context())
			}
		},
	}
}
