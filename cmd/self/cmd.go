// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package self

import (
	"github.com/datarobot/cli/cmd/self/config"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "self",
		GroupID: "self",
		Short:   "Run DataRobot CLI utility commands.",
	}

	cmd.AddCommand(
		CompletionCmd(),
		config.Cmd(),
		UpdateCmd(),
		VersionCmd(),
	)

	return cmd
}
