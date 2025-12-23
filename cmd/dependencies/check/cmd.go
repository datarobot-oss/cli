// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package check

import (
	"errors"

	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check template dependencies.",
		RunE:  RunE,
	}

	return cmd
}

func RunE(cmd *cobra.Command, _ []string) error {
	cleanup := tui.SetupDebugLogging()
	defer cleanup()

	missing := tools.MissingPrerequisites()

	if missing != "" {
		cmd.SilenceUsage = true
		return errors.New(missing)
	}

	return nil
}
