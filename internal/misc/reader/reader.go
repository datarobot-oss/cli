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

package reader

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

func ReadString() (string, error) {
	if runtime.GOOS == "windows" {
		return bufio.NewReader(os.Stdin).ReadString('\n')
	}

	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return "", err
	}

	cancelChan := make(chan os.Signal, 1)
	defer close(cancelChan)

	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(cancelChan)

	go func() {
		<-cancelChan
		cr.Cancel()
	}()

	reader := bufio.NewReader(cr)

	str, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println()
	}

	return str, err
}

// AskYesNo prints nothing itself — the caller is expected to have already
// prompted the user. It reads one line from stdin and returns true unless
// the user explicitly types "n" or "no" (case-insensitive).
// An empty input (just pressing Enter) is treated as yes.
// Any read error (including Ctrl+C / SIGINT cancellation) is treated as no.
func AskYesNo() bool {
	line, err := ReadString()
	if err != nil {
		return false
	}

	answer := strings.TrimSpace(strings.ToLower(line))

	return answer != "n" && answer != "no"
}

// IsStdinTerminal reports whether stdin is connected to an interactive terminal.
// Returns false when stdin is a pipe, a file redirect, or otherwise non-interactive.
func IsStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// NonInteractiveEnv is the env var users set to force non-interactive mode
// (e.g. Agent Assist). It is also bound to the viper "yes" key in commands
// that support a --yes flag.
const NonInteractiveEnv = "DATAROBOT_CLI_NON_INTERACTIVE"

// IsNonInteractive reports whether DATAROBOT_CLI_NON_INTERACTIVE is set to a
// truthy value. Callers should use this to skip animations, prompts, and other
// interactive UI when running under automation.
func IsNonInteractive() bool {
	switch os.Getenv(NonInteractiveEnv) {
	case "1", "t", "T", "true", "TRUE", "True", "y", "Y", "yes", "YES", "Yes":
		return true
	}

	return false
}
