// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package main

import (
	"log"
	"os"

	"github.com/datarobot/cli/cmd"
	"github.com/datarobot/cli/tui"
)

func runInteractiveMode() {
	tui.Start()
}

func runNonInteractiveMode() {
	if err := cmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func main() {
	// If no arguments (besides the program name itself) are passed,
	// start the interactive TUI.
	if len(os.Args) == 1 {
		runInteractiveMode()
	} else {
		// Otherwise, execute the command-line interface with the provided arguments.
		runNonInteractiveMode()
	}
}
