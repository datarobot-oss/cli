// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/datarobot/cli/tui"
)

type Shell string

const (
	Bash       Shell = "bash"
	Zsh        Shell = "zsh"
	Fish       Shell = "fish"
	PowerShell Shell = "powershell"
)

func DetectShell() (string, error) {
	// Try SHELL environment variable first
	shellPath := os.Getenv("SHELL")
	if shellPath != "" {
		return filepath.Base(shellPath), nil
	}

	// On Windows, check for PowerShell
	if runtime.GOOS == "windows" {
		return string(PowerShell), nil
	}

	return "", errors.New("could not detect shell. Please set SHELL environment variable")
}

func ResolveShell(specifiedShell string) (string, error) {
	if specifiedShell != "" {
		// Use specified shell
		fmt.Printf("%s Installing for shell: %s\n", tui.InfoStyle.Render("→"), specifiedShell)

		return specifiedShell, nil
	}

	// Detect current shell
	shell, err := DetectShell()
	if err != nil {
		return "", err
	}

	fmt.Printf("%s Detected shell: %s\n", tui.InfoStyle.Render("→"), shell)

	return shell, nil
}
