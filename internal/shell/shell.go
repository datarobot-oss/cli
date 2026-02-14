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

func SupportedShells() []string {
	return []string{
		string(Bash),
		string(Zsh),
		string(Fish),
		string(PowerShell),
	}
}

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

	return "", errors.New("Could not detect shell. Please set SHELL environment variable")
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
