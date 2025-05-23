// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package main

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/cmd"
	"github.com/datarobot/cli/tui"
)

func runInteractiveMode() error {
	return tui.Start()
}

func runNonInteractiveMode() error {
	return cmd.Execute()
}

func main() {
	var err error
	// If no arguments (besides the program name itself) are passed,
	// start the interactive TUI.
	if len(os.Args) == 1 {
		err = runInteractiveMode()
	} else {
		// Otherwise, execute the command-line interface with the provided arguments.
		err = runNonInteractiveMode()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
