// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package open

import (
	"os/exec"
	"runtime"
	"testing"
)

func Open(url string) {
	if testing.Testing() {
		return
	}

	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Run()
	case "windows":
		_ = exec.Command("start", url).Run()
	default:
		_ = exec.Command("xdg-open", url).Run()
	}
}
