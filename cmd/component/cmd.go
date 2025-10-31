// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"github.com/datarobot/cli/cmd/component/add"
	"github.com/datarobot/cli/cmd/component/list"
	"github.com/datarobot/cli/cmd/component/update"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "component",
		GroupID: "core",
		Short:   "Manage components",
	}

	cmd.AddCommand(
		add.Cmd(),
		list.Cmd(),
		update.Cmd(),
	)

	return cmd
}
