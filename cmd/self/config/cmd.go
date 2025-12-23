// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Display current configuration settings",
		Long:  "Display all configuration settings from config file and environment variables, with sensitive data redacted.",
		RunE:  RunE,
	}

	return cmd
}

func RunE(cmd *cobra.Command, _ []string) error {
	output, err := config.DebugViperConfig()
	if err != nil {
		return err
	}

	cmd.Print(output)

	return nil
}
