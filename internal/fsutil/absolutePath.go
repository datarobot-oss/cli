// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

func AbsolutePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		path = strings.Replace(path, "~/", "$HOME/", 1)
	}

	path = os.ExpandEnv(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	return absPath
}
