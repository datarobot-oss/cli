// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package update

import (
	"fmt"

	//"github.com/datarobot/cli/cmd/task/compose"
	"github.com/spf13/cobra"
)

func Run(_ *cobra.Command, _ []string) {
	fmt.Println("dr component update")

	// compose.Run(nil, nil)
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update component",
		Run:   Run,
	}

	return cmd
}
