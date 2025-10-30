// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package add

import (
	"fmt"

	"github.com/spf13/cobra"
)

func Run(_ *cobra.Command, _ []string) {
	fmt.Println("dr component add")
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add component",
		Run:   Run,
	}

	return cmd
}
