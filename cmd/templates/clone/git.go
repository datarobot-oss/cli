// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package clone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitClone(repoURL, dir string) (string, error) {
	cmd := exec.Command("git", "clone", repoURL, dir)

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}

func gitOrigin(dir string, isAbsolute bool) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")

	path, err := os.Getwd()
	if err != nil {
		return ""
	}

	if isAbsolute {
		cmd.Dir = dir
	} else {
		cmd.Dir = filepath.Join(path, dir)
	}

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(stdout))
}

func gitPull(dir string) (string, error) {
	cmd := exec.Command("git", "pull")

	path, err := os.Getwd()
	if err != nil {
		return "", err
	}

	cmd.Dir = filepath.Join(path, dir)

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}
