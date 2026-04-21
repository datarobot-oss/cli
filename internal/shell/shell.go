// Copyright 2026 DataRobot, Inc. and its affiliates.
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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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
	// Prefer the parent process name — accurate even after `exec sh` or similar.
	if name := parentProcessName(); name != "" {
		return name, nil
	}

	// Try SHELL environment variable next.
	if shellPath := os.Getenv("SHELL"); shellPath != "" {
		return filepath.Base(shellPath), nil
	}

	// On Windows, default to PowerShell.
	if runtime.GOOS == "windows" {
		return string(PowerShell), nil
	}

	return "", errors.New("Could not detect shell. Please set SHELL environment variable")
}

// parentProcessName returns the short name of the process that launched the
// CLI (i.e. the running shell). On Linux it reads /proc/{ppid}/comm; on
// macOS and other Unix systems it falls back to querying ps. Returns an
// empty string when the name cannot be determined.
func parentProcessName() string {
	ppid := os.Getppid()

	// Linux: /proc/{ppid}/comm contains the short process name.
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(ppid) + "/comm"); err == nil {
		return strings.TrimSpace(string(data))
	}

	// macOS and other Unix: ask ps for the command name.
	out, err := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}

	return filepath.Base(strings.TrimSpace(string(out)))
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
