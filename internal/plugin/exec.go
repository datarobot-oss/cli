// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// ExecutePlugin runs a plugin and returns its exit code
func ExecutePlugin(executable string, args []string) int {
	cmd := exec.Command(executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Forward signals to child process with cleanup
	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case <-done:
			// Command completed, exit goroutine cleanly
			return
		}
	}()

	err := cmd.Run()

	// Signal goroutine to exit and cleanup
	close(done)
	signal.Stop(sigChan)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		return 1
	}

	return 0
}
