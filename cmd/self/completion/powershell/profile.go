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

// Package powershell holds what `completion install` and `completion uninstall` share about the
// PowerShell profile: the profile paths, the command the profile runs, and the blocks the CLI
// writes into it. The block writer and the block remover live together so they stay a pair.
package powershell

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/version"
)

const profileFileName = "Microsoft.PowerShell_profile.ps1"

// ErrNotUTF8 reports a profile that byte-level edits cannot put back soundly, such as the UTF-16
// file that `echo > $PROFILE` writes in Windows PowerShell 5.1.
var ErrNotUTF8 = errors.New("profile is not UTF-8 text")

var (
	// Command is what a profile block runs to load completion.
	Command = version.CliName + " self completion powershell"
	// LegacyCommand is what earlier releases wrote. It is not a command, so every session that
	// loaded such a profile printed an error.
	LegacyCommand = version.CliName + " completion powershell"
)

// ProfilePaths returns every profile file a PowerShell session can load for the user. Windows has
// one for PowerShell 7 and one for Windows PowerShell 5.1. Other platforms have one.
func ProfilePaths() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get user home dir: %w", err)
	}

	if runtime.GOOS != "windows" {
		return []string{filepath.Join(homeDir, ".config", "powershell", profileFileName)}, nil
	}

	documentsPath := filepath.Join(homeDir, "Documents")

	return []string{
		filepath.Join(documentsPath, "PowerShell", profileFileName),
		filepath.Join(documentsPath, "WindowsPowerShell", profileFileName),
	}, nil
}

// InstallProfilePath returns the profile a fresh install writes: the PowerShell 7 profile when its
// folder exists, otherwise the Windows PowerShell profile.
func InstallProfilePath() (string, error) {
	paths, err := ProfilePaths()
	if err != nil {
		return "", err
	}

	if len(paths) == 1 || fsutil.DirExists(filepath.Dir(paths[0])) {
		return paths[0], nil
	}

	return paths[1], nil
}

// CheckEditable returns ErrNotUTF8 when the profile bytes are UTF-16, which an edit that adds or
// drops UTF-8 text would corrupt.
func CheckEditable(data []byte) error {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) || bytes.IndexByte(data, 0) >= 0 {
		return ErrNotUTF8
	}

	return nil
}

// MatchLineEndings gives generated text the line endings the profile already uses.
func MatchLineEndings(existing, generated string) string {
	if strings.Contains(existing, "\r\n") {
		return strings.ReplaceAll(generated, "\n", "\r\n")
	}

	return generated
}

func marker(name string) string {
	return "# " + name + " completion"
}

func aliasMarker(alias string) string {
	return "# " + alias + " alias completion"
}

func guardLine(name string) string {
	return fmt.Sprintf("if (Get-Command %s -ErrorAction SilentlyContinue) {", name)
}

// MissingBlocks returns the completion blocks the profile content lacks.
func MissingBlocks(existingContent string) string {
	var sb strings.Builder

	if !strings.Contains(existingContent, Command) {
		fmt.Fprintf(&sb, "\n%s\n", marker(version.CliName))
		fmt.Fprintf(&sb, "%s\n", guardLine(version.CliName))
		fmt.Fprintf(&sb, "    %s | Out-String | Invoke-Expression\n", Command)
		sb.WriteString("}\n")
	}

	for _, alias := range version.CliAliases {
		if !strings.Contains(existingContent, aliasMarker(alias)) {
			fmt.Fprintf(&sb, "\n%s\n", aliasMarker(alias))
			fmt.Fprintf(&sb, "%s\n", guardLine(alias))
			fmt.Fprintf(&sb, "    $__drCompletionScript = (%s | Out-String)\n", Command)
			fmt.Fprintf(&sb, "    Invoke-Expression ($__drCompletionScript -replace \"CommandName '%s'\", \"CommandName '%s'\")\n", version.CliName, alias)
			sb.WriteString("}\n")
		}
	}

	return sb.String()
}

// blockEnd returns the index after the closing brace of the block that starts at lines[start], or
// -1 when the lines there are not a block MissingBlocks wrote. It matches the marker and the guard
// line exactly, so a comment or a line the user wrote is never mistaken for a block.
func blockEnd(lines []string, start int) int {
	name, ok := markerName(strings.TrimSpace(lines[start]))
	if !ok || start+1 >= len(lines) || strings.TrimRight(lines[start+1], "\r") != guardLine(name) {
		return -1
	}

	for i := start + 2; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "}" {
			return i + 1
		}
	}

	return -1
}

// markerName returns the command a marker line belongs to.
func markerName(line string) (string, bool) {
	if line == marker(version.CliName) {
		return version.CliName, true
	}

	for _, alias := range version.CliAliases {
		if line == aliasMarker(alias) {
			return alias, true
		}
	}

	return "", false
}

// MigrateLegacy replaces the legacy command inside the blocks the CLI wrote and reports whether it
// changed anything. Text outside those blocks stays as it is.
func MigrateLegacy(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	changed := false

	for i := 0; i < len(lines); i++ {
		end := blockEnd(lines, i)
		if end < 0 {
			continue
		}

		for j := i; j < end; j++ {
			if migrated := strings.ReplaceAll(lines[j], LegacyCommand, Command); migrated != lines[j] {
				lines[j] = migrated
				changed = true
			}
		}

		i = end - 1
	}

	return strings.Join(lines, "\n"), changed
}

// RemoveBlocks drops the blocks the CLI wrote, each from its marker through its closing brace, and
// the blank line before it. Text outside those blocks stays as it is.
func RemoveBlocks(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		end := blockEnd(lines, i)
		if end < 0 {
			kept = append(kept, lines[i])

			continue
		}

		if len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
			kept = kept[:len(kept)-1]
		}

		i = end - 1
	}

	return strings.Join(kept, "\n")
}
