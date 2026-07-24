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
		// ok is false only when cancelChan was closed (normal completion)
		// with nothing pending — not from a genuine signal. Without this
		// check, every successful read would still call cr.Cancel() here
		// (close() unblocks a pending receive same as a real send would),
		// racing the deferred cr.Close() above: cancelreader has no
		// synchronization between Cancel() and Close(), and on Windows
		// Close() skips console-mode restoration entirely if the race makes
		// CloseHandle(cancelEvent) fail — the exact bug this file fixes.
		if _, ok := <-cancelChan; ok {
			cr.Cancel()
		}
	}()

	reader := bufio.NewReader(cr)

	str, err := readLine(reader)
	if err != nil {
		fmt.Println()
	}

	return str, err
}

const (
	ctrlC     = 0x03 // ASCII ETX, sent by Ctrl+C
	backspace = '\b'
	del       = 0x7f // ASCII DEL, sent by Backspace on some terminals; also the exclusive upper bound of printable ASCII
	esc       = 0x1b // ASCII ESC, begins VT100/ANSI escape sequences (arrows, Home/End, etc.)

	printableASCIIMin = 0x20 // ' ', the lowest printable ASCII byte

	eraseSequence = "\b \b" // backspace, overwrite with space, backspace again

	csiFinalByteMin = 0x40 // '@', low end of the CSI final-byte range (ECMA-48)
	csiFinalByteMax = 0x7e // '~', high end of the CSI final-byte range (ECMA-48)
)

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

		// Ctrl+C and escape sequences (arrow keys, Home/End, etc.) never
		// belong in the answer — see handleControlByte.
		if consumed, err := handleControlByte(r, b); consumed {
			if err != nil {
				return sb.String(), err
			}

			continue
		}

		if isLineEnd(b) {
			if echo {
				fmt.Println()
			}

			return sb.String(), nil
		}

		if isBackspace(b) {
			if echo {
				eraseLastByte(&sb)
			}

			continue
		}

		sb.WriteByte(b)

		if echo {
			echoByte(b)
		}
	}
}

// isLineEnd reports whether b is '\n' or '\r'.
func isLineEnd(b byte) bool {
	return b == '\n' || b == '\r'
}

// handleControlByte handles the two byte values that never belong in the
// answer text itself. consumed is true if b was one of them, in which case
// the caller should `continue` its read loop when err is nil, or stop and
// return err otherwise.
//
//   - Ctrl+C (ASCII ETX, 0x03): on POSIX this byte never reaches us — the tty
//     driver intercepts it at the ISIG layer and raises a real SIGINT before
//     it's ever placed in the input stream. On Windows, cancelreader disables
//     ENABLE_PROCESSED_INPUT (required so it can implement its own Cancel()),
//     which per Microsoft's docs means "CTRL+C is reported as keyboard input
//     rather than as a signal" — so this is the only place Ctrl+C is ever
//     observable on Windows.
//   - Escape sequences (ESC, 0x1b): arrow keys, Home/End, Delete, etc. are
//     sent as VT100/ANSI escape sequences (e.g. Right arrow is ESC '[' 'C')
//     on every platform — this is how terminals encode special keys,
//     independent of cooked vs. raw console/tty mode. Without discarding
//     these, the bytes get echoed and appended into the answer as literal
//     control characters (e.g. a typed URL ending up as
//     "abcdefghi\x1b[C\x1b[C" and failing url.Parse downstream). We don't yet
//     act on them (e.g. actually moving the cursor) — just discard them so
//     they can't corrupt the answer.
func handleControlByte(r *bufio.Reader, b byte) (consumed bool, err error) {
	switch b {
	case ctrlC:
		return true, cancelreader.ErrCanceled
	case esc:
		return true, discardEscapeSequence(r)
	}

	return false, nil
}

// isBackspace reports whether b is a backspace or DEL byte.
func isBackspace(b byte) bool {
	return b == backspace || b == del
}

// discardEscapeSequence consumes and discards a VT100/ANSI escape sequence
// following an ESC byte already read from r, so it isn't echoed or appended
// to the answer as literal control bytes (e.g. arrow keys send ESC '[' 'C'/'D').
// A CSI sequence is ESC '[' followed by any number of parameter/intermediate
// bytes and a single final byte in the 0x40-0x7e range (ECMA-48).
// If the byte after ESC isn't '[', this wasn't a CSI sequence (e.g. a bare
// Escape keypress) — that byte is pushed back via UnreadByte so the caller
// processes it normally instead of silently swallowing an unrelated keystroke.
func discardEscapeSequence(r *bufio.Reader) error {
	next, err := r.ReadByte()
	if err != nil {
		return err
	}

	if next != '[' {
		return r.UnreadByte()
	}

	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}

		if b >= csiFinalByteMin && b <= csiFinalByteMax {
			return nil
		}
	}
}

// eraseLastByte removes the last byte from sb, if any, and erases its
// on-screen representation via the standard backspace-space-backspace trick.
func eraseLastByte(sb *strings.Builder) {
	s := sb.String()
	if s == "" {
		return
	}

	sb.Reset()
	sb.WriteString(s[:len(s)-1])
	fmt.Print(eraseSequence)
}

// echoByte prints b if it falls in the printable ASCII range.
func echoByte(b byte) {
	if b >= printableASCIIMin && b < del {
		fmt.Printf("%c", b)
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
