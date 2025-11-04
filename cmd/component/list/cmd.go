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
	"os"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/copier"
	"github.com/spf13/cobra"
)

func RunE(_ *cobra.Command, _ []string) error {
	answers, err := copier.AnswersFromPath(".")
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Answers file\tRepository\n")

	for _, answer := range answers {
		fmt.Fprintf(w, "%s\t%s\n", answer.FileName, answer.Repo)
	}

	w.Flush()

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
