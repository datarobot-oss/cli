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

func Add(repoURL string) *exec.Cmd {
	return exec.Command("uvx", "copier", "copy", repoURL, ".")
}

func ExecAdd(repoURL string) error {
	if repoURL == "" {
		return errors.New("Repository URL is missing.")
	}

	cmd := Add(repoURL)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func Update(yamlFile string) *exec.Cmd {
	return exec.Command("uvx", "copier", "update", "-a", yamlFile, "-A")
}

func ExecUpdate(yamlFile string) error {
	if yamlFile == "" {
		return errors.New("Path to YAML file is missing.")
	}

	cmd := Update(yamlFile)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}
