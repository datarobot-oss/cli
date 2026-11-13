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
	"regexp"
	"runtime"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/version"
)

const profileFileName = "Microsoft.PowerShell_profile.ps1"

// ErrNotUTF8 reports a profile that byte-level edits cannot put back soundly, such as the UTF-16
// file that `echo > $PROFILE` writes in Windows PowerShell 5.1.
var ErrNotUTF8 = errors.New("profile is not UTF-8 text")

// LegacyCommand is what earlier releases wrote into the profile. It is not a command, so every
// session that loaded such a profile printed an error. The current command comes from the command
// tree, so callers pass it in.
var LegacyCommand = version.CliName + " completion powershell"

// printsScript matches the CLI's command that prints the PowerShell completion script, in the
// current and in the legacy form. It does not match another tool's `... completion powershell`,
// such as `kubectl completion powershell`.
var printsScript = regexp.MustCompile(`(?m)(?:^|[^\w-])` + regexp.QuoteMeta(version.CliName) + `(?: self)? completion powershell\b`)

// LoadsCompletion reports whether the profile content runs the CLI's command that prints the
// completion script, whoever wrote the line.
func LoadsCompletion(content string) bool {
	return printsScript.MatchString(content)
}

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

// MissingBlocks returns the completion blocks the profile content lacks. The blocks run command,
// for example `dr self completion powershell`.
func MissingBlocks(existingContent, command string) string {
	var sb strings.Builder

	if !strings.Contains(withoutAliasBlocks(existingContent), command) {
		fmt.Fprintf(&sb, "\n%s\n", marker(version.CliName))
		fmt.Fprintf(&sb, "%s\n", guardLine(version.CliName))
		fmt.Fprintf(&sb, "    %s | Out-String | Invoke-Expression\n", command)
		sb.WriteString("}\n")
	}

	for _, alias := range version.CliAliases {
		if !strings.Contains(existingContent, aliasMarker(alias)) {
			fmt.Fprintf(&sb, "\n%s\n", aliasMarker(alias))
			fmt.Fprintf(&sb, "%s\n", guardLine(alias))
			fmt.Fprintf(&sb, "    $__drCompletionScript = (%s | Out-String)\n", command)
			fmt.Fprintf(&sb, "    Invoke-Expression ($__drCompletionScript -replace \"CommandName '%s'\", \"CommandName '%s'\")\n", version.CliName, alias)
			sb.WriteString("}\n")
		}
	}

	return sb.String()
}

// bodyShape returns the prefix and suffix of each body line MissingBlocks writes in a block. The
// command in the middle varies, so a line is matched by its ends.
func bodyShape(name string) [][2]string {
	if name == version.CliName {
		return [][2]string{{"    ", " | Out-String | Invoke-Expression"}}
	}

	return [][2]string{
		{"    $__drCompletionScript = (", " | Out-String)"},
		{"    Invoke-Expression ($__drCompletionScript -replace ", ")"},
	}
}

// blockEnd returns the index after the closing brace of the block that starts at lines[start], or
// -1 when the lines there are not a block MissingBlocks wrote. The marker, the guard line, every
// body line and the closing brace must sit where MissingBlocks puts them, so a comment, a re-indented
// brace or a line the user wrote is never mistaken for a block, and a search never runs on into the
// user's code.
func blockEnd(lines []string, start int) int {
	name, ok := markerName(strings.TrimSpace(lines[start]))
	if !ok {
		return -1
	}

	shape := bodyShape(name)
	end := start + 2 + len(shape) + 1

	if end > len(lines) || strings.TrimRight(lines[start+1], "\r") != guardLine(name) || strings.TrimRight(lines[end-1], "\r") != "}" {
		return -1
	}

	for i, affixes := range shape {
		line := strings.TrimRight(lines[start+2+i], "\r")
		if !strings.HasPrefix(line, affixes[0]) || !strings.HasSuffix(line, affixes[1]) {
			return -1
		}
	}

	return end
}

// withoutAliasBlocks drops the alias blocks. Their body runs the command too, so a check for the
// main block must not see it.
func withoutAliasBlocks(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		if name, ok := markerName(strings.TrimSpace(lines[i])); ok && name != version.CliName {
			if end := blockEnd(lines, i); end > 0 {
				i = end - 1

				continue
			}
		}

		kept = append(kept, lines[i])
	}

	return strings.Join(kept, "\n")
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

// MigrateLegacy replaces the legacy command inside the blocks the CLI wrote with command and
// reports whether it changed anything. Text outside those blocks stays as it is.
func MigrateLegacy(content, command string) (string, bool) {
	lines := strings.Split(content, "\n")
	changed := false

	for i := 0; i < len(lines); i++ {
		end := blockEnd(lines, i)
		if end < 0 {
			continue
		}

		for j := i; j < end; j++ {
			if migrated := strings.ReplaceAll(lines[j], LegacyCommand, command); migrated != lines[j] {
				lines[j] = migrated
				changed = true
			}
		}

		i = end - 1
	}

	return strings.Join(lines, "\n"), changed
}

// RemoveBlocks drops the blocks the CLI wrote, each from its marker through its closing brace, and
// the blank line before it, and gives back the profile as it was before they were added. Text
// outside those blocks stays as it is.
func RemoveBlocks(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))

	// keptAfterRemoval is len(kept) right after the last removed block. It tells a run of adjacent
	// blocks from blocks that user text separates.
	keptAfterRemoval := -1
	runFollowsText := false

	for i := 0; i < len(lines); i++ {
		end := blockEnd(lines, i)
		if end < 0 {
			kept = append(kept, lines[i])

			continue
		}

		popped := len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == ""
		if popped {
			kept = kept[:len(kept)-1]
		}

		if len(kept) != keptAfterRemoval {
			// Install writes a blank line before its first block. A block that follows text with no
			// blank line began on a profile whose last line had no line break, so the break is part of
			// what install added.
			runFollowsText = !popped && len(kept) > 0
		}

		keptAfterRemoval = len(kept)
		i = end - 1
	}

	// The blocks end the profile when only the final line break follows the last one.
	if runFollowsText && len(kept) == keptAfterRemoval+1 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
		kept[len(kept)-1] = strings.TrimSuffix(kept[len(kept)-1], "\r")
	}

	return strings.Join(kept, "\n")
}
