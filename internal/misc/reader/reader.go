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

	"github.com/datarobot/cli/internal/config"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

func ReadString() (string, error) {
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return "", err
	}

	// cancelreader must be closed. That never happened here: leaks epoll/pipe
	// fds on Linux/macOS, and leaves Windows in raw console mode (no echo) —
	// the garbled input CFX-4799 papered over by bypassing cancelreader on
	// Windows instead of closing it.
	defer cr.Close()

	cancelChan := make(chan os.Signal, 1)
	defer close(cancelChan)

	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(cancelChan)

	go func() {
		<-cancelChan
		cr.Cancel()
	}()

	reader := bufio.NewReader(cr)

	str, err := readLine(reader)
	if err != nil {
		fmt.Println("readLine err => : ", err)
		fmt.Println()
	}

	return str, err
}

// readLine reads bytes until '\n' or '\r', whichever comes first, without
// looking ahead for a paired '\r\n'. That lookahead would block: with
// ENABLE_LINE_INPUT off, Windows' raw console mode delivers a bare '\r' for
// Enter and nothing more until the next keystroke, so waiting to see whether
// '\n' follows would hang until the user types again. POSIX terminals
// already translate Enter to a bare '\n' at the tty layer, so they never hit
// the '\r' branch here. This also means redirected/piped input is handled
// regardless of its line-ending convention ('\n', '\r', or '\r\n').
func readLine(r *bufio.Reader) (string, error) {
	var sb strings.Builder

	// cancelreader's Windows raw console mode also disables ENABLE_ECHO_INPUT,
	// so the console stops echoing typed characters (and doesn't erase them
	// on backspace) on its own — we have to do both ourselves there. POSIX
	// never touches termios, so the tty keeps echoing normally on its own;
	// echoing here too would double every character.
	echo := runtime.GOOS == "windows"

	for {
		b, err := r.ReadByte()
		if err != nil {
			return sb.String(), err
		}

		// Ctrl+C (ASCII ETX, 0x03). On POSIX this byte never reaches us —
		// the tty driver intercepts it at the ISIG layer and raises a real
		// SIGINT before it's ever placed in the input stream. On Windows,
		// cancelreader disables ENABLE_PROCESSED_INPUT (required so it can
		// implement its own Cancel()), which per Microsoft's docs means
		// "CTRL+C is reported as keyboard input rather than as a signal" —
		// so this is the only place Ctrl+C is ever observable on Windows.
		if b == 0x03 {
			fmt.Println("ReadByte 0x03", err)
			return sb.String(), cancelreader.ErrCanceled
		}

		if b == '\n' || b == '\r' {
			if echo {
				fmt.Println()
			}

			return sb.String(), nil
		}

		if echo && (b == '\b' || b == 0x7f) {
			if s := sb.String(); s != "" {
				sb.Reset()
				sb.WriteString(s[:len(s)-1])
				fmt.Print("\b \b")
			}

			continue
		}

		sb.WriteByte(b)

		if echo && b >= 0x20 && b < 0x7f {
			fmt.Printf("%c", b)
		}
	}
}

// AskYesNo prints nothing itself — the caller is expected to have already
// prompted the user. It reads one line from stdin and returns true unless
// the user explicitly types "n" or "no" (case-insensitive).
// An empty input (just pressing Enter) is treated as yes.
// Any read error (including Ctrl+C / SIGINT cancellation) is treated as no.
func AskYesNo() bool {
	line, err := ReadString()
	if err != nil {
		fmt.Println("err", err)
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
const NonInteractiveEnv = config.EnvPrefix + "NON_INTERACTIVE"

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
