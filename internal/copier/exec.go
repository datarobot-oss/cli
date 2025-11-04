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

func ExecAdd(repoURL string) error {
	if repoURL == "" {
		return errors.New("repository URL is missing")
	}

	cmd := exec.Command("uvx", "copier", "copy", repoURL, ".")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func ExecUpdate(yamlFile string) error {
	if yamlFile == "" {
		return errors.New("path to yaml file is missing")
	}

	cmd := exec.Command("uvx", "copier", "update", "-a", yamlFile, "-A")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}
