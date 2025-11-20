// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package copier

import (
	"errors"
	"os"
	"os/exec"
)

func cmdRun(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func Add(repoURL string) *exec.Cmd {
	return exec.Command("uvx", "copier", "copy", repoURL, ".")
}

func ExecAdd(repoURL string) error {
	if repoURL == "" {
		return errors.New("Repository URL is missing.")
	}

	return cmdRun(Add(repoURL))
}

func Update(yamlFile string, recopy, quiet, debug bool) *exec.Cmd {
	copierCommand := "update"

	if recopy {
		copierCommand = "recopy"
	}

	commandParts := []string{
		"copier", copierCommand, "--answers-file", yamlFile, "--skip-answered",
	}
	if quiet {
		commandParts = append(commandParts, "--quiet")
	}

	cmd := exec.Command("uvx", commandParts...)

	// Suppress all Python warnings unless debug mode is enabled
	if !debug {
		cmd.Env = append(os.Environ(), "PYTHONWARNINGS=ignore")
	}

	return cmd
}

func ExecUpdate(yamlFile string, recopy, quiet, debug bool) error {
	if yamlFile == "" {
		return errors.New("Path to YAML file is missing.")
	}

	return cmdRun(Update(yamlFile, recopy, quiet, debug))
}
