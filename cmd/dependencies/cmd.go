// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dependencies

import (
	"github.com/datarobot/cli/cmd/dependencies/check"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dependencies",
		GroupID: "advanced",
		Short:   "Commands related to template dependencies.",
	}

	cmd.AddCommand(
		check.Cmd(),
	)

	return cmd
}
