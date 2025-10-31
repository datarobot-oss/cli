// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"fmt"

	"github.com/datarobot/cli/internal/copier"

	"github.com/spf13/cobra"
)

func RunE(_ *cobra.Command, _ []string) error {
	fmt.Println("dr component list")

	answers, err := copier.AnswersFromPath(".")
	if err != nil {
		return err
	}

	for _, answer := range answers {
		fmt.Println(answer.FileName, answer.Repo)
	}

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List components",
		RunE:  RunE,
	}

	return cmd
}
