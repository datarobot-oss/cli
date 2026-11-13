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

package powershell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	command     = "dr self completion powershell"
	userLine    = "Set-Alias ll Get-ChildItem\n"
	legacyBlock = "\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"
)

func TestMissingBlocks(t *testing.T) {
	t.Run("empty content returns every block", func(t *testing.T) {
		got := MissingBlocks("", command)

		assert.Contains(t, got, "# dr completion")
		assert.Contains(t, got, "dr self completion powershell | Out-String | Invoke-Expression")
		assert.Contains(t, got, "# datarobot alias completion")
	})

	t.Run("an existing dr block skips dr but keeps the alias", func(t *testing.T) {
		got := MissingBlocks("dr self completion powershell | Out-String | Invoke-Expression\n", command)

		assert.NotContains(t, got, "# dr completion")
		assert.Contains(t, got, "# datarobot alias completion")
	})

	t.Run("an existing alias block skips the alias", func(t *testing.T) {
		got := MissingBlocks("# datarobot alias completion\n", command)

		assert.NotContains(t, got, "# datarobot alias completion")
		assert.Contains(t, got, "dr self completion powershell")
	})

	t.Run("every block present returns nothing", func(t *testing.T) {
		got := MissingBlocks("dr self completion powershell | Out-String | Invoke-Expression\n# datarobot alias completion\n", command)

		assert.Empty(t, got)
	})
}

func TestMigrateLegacy(t *testing.T) {
	t.Run("a legacy dr block is migrated", func(t *testing.T) {
		got, migrated := MigrateLegacy(userLine+legacyBlock, command)

		assert.True(t, migrated)
		assert.Contains(t, got, "    dr self completion powershell | Out-String | Invoke-Expression\n")
		assert.NotContains(t, got, "dr completion powershell")
		assert.Contains(t, got, userLine)
	})

	t.Run("a legacy alias block is migrated", func(t *testing.T) {
		content := "\n# datarobot alias completion\nif (Get-Command datarobot -ErrorAction SilentlyContinue) {\n    $__drCompletionScript = (dr completion powershell | Out-String)\n    Invoke-Expression ($__drCompletionScript -replace \"CommandName 'dr'\", \"CommandName 'datarobot'\")\n}\n"

		got, migrated := MigrateLegacy(content, command)

		assert.True(t, migrated)
		assert.Contains(t, got, "(dr self completion powershell | Out-String)")
		assert.NotContains(t, got, "dr completion powershell")
	})

	t.Run("a CRLF block keeps its line endings", func(t *testing.T) {
		got, migrated := MigrateLegacy(strings.ReplaceAll(legacyBlock, "\n", "\r\n"), command)

		assert.True(t, migrated)
		assert.Equal(t, strings.ReplaceAll(strings.ReplaceAll(legacyBlock, "dr completion powershell", "dr self completion powershell"), "\n", "\r\n"), got)
	})

	tests := []struct {
		name    string
		content string
	}{
		{"a current block is left alone", "\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr self completion powershell | Out-String | Invoke-Expression\n}\n"},
		{"unrelated content is left alone", userLine},
		{"a similar command outside a block is left alone", "mydr completion powershell\n"},
		{"the legacy command in a user line is left alone", "# run dr completion powershell by hand\n" + userLine},
		{"a marker without the guard line is left alone", "# dr completion\nWrite-Host 'dr completion powershell'\n"},
		{"a block without a closing brace is left alone", "# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, migrated := MigrateLegacy(tt.content, command)

			assert.False(t, migrated)
			assert.Equal(t, tt.content, got)
		})
	}
}

func TestMissingBlocksRunsTheCommandItIsGiven(t *testing.T) {
	got := MissingBlocks("", "tool sub completion powershell")

	assert.Contains(t, got, "    tool sub completion powershell | Out-String | Invoke-Expression\n")
	assert.Contains(t, got, "$__drCompletionScript = (tool sub completion powershell | Out-String)")
	assert.NotContains(t, got, "dr self completion powershell")
}

func TestMigrateLegacyWritesTheCommandItIsGiven(t *testing.T) {
	got, migrated := MigrateLegacy(userLine+legacyBlock, "tool sub completion powershell")

	assert.True(t, migrated)
	assert.Contains(t, got, "    tool sub completion powershell | Out-String | Invoke-Expression\n")
	assert.NotContains(t, got, "dr completion powershell")
}

func TestLoadsCompletion(t *testing.T) {
	assert.True(t, LoadsCompletion("dr self completion powershell | Out-String | Invoke-Expression\n"), "the current form")
	assert.True(t, LoadsCompletion("dr completion powershell | Out-String | Invoke-Expression\n"), "the legacy form")
	assert.False(t, LoadsCompletion(userLine), "an unrelated profile")
}

func TestRemoveBlocks(t *testing.T) {
	t.Run("the blocks MissingBlocks wrote are removed in full, alias block included", func(t *testing.T) {
		installed := userLine + MissingBlocks(userLine, command)

		assert.Equal(t, userLine, RemoveBlocks(installed))
	})

	t.Run("an alias block alone leaves no stray closing brace", func(t *testing.T) {
		got := RemoveBlocks(userLine + "\n# datarobot alias completion\nif (Get-Command datarobot -ErrorAction SilentlyContinue) {\n    $__drCompletionScript = (dr self completion powershell | Out-String)\n    Invoke-Expression ($__drCompletionScript -replace \"CommandName 'dr'\", \"CommandName 'datarobot'\")\n}\n")

		assert.Equal(t, userLine, got)
	})

	t.Run("a legacy block is removed", func(t *testing.T) {
		assert.Equal(t, userLine, RemoveBlocks(userLine+legacyBlock))
	})

	t.Run("the user's lines around a block stay", func(t *testing.T) {
		got := RemoveBlocks(userLine + legacyBlock + "Set-Alias gs git-status\n")

		assert.Equal(t, userLine+"Set-Alias gs git-status\n", got)
	})

	t.Run("a CRLF profile is removed without bare line feeds", func(t *testing.T) {
		installed := strings.ReplaceAll(userLine+MissingBlocks(userLine, command), "\n", "\r\n")

		assert.Equal(t, strings.ReplaceAll(userLine, "\n", "\r\n"), RemoveBlocks(installed))
	})

	t.Run("a line the user wrote is left alone", func(t *testing.T) {
		content := userLine + "dr self completion powershell | Out-String | Invoke-Expression\n"

		assert.Equal(t, content, RemoveBlocks(content))
	})

	t.Run("a marker without the guard line is left alone", func(t *testing.T) {
		content := "# dr completion\nWrite-Host 'mine'\n}\n"

		assert.Equal(t, content, RemoveBlocks(content))
	})

	t.Run("a block without a closing brace is left alone", func(t *testing.T) {
		content := userLine + "# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr self completion powershell\n"

		assert.Equal(t, content, RemoveBlocks(content))
	})

	t.Run("a profile without blocks is unchanged", func(t *testing.T) {
		assert.Equal(t, userLine, RemoveBlocks(userLine))
	})
}

func TestCheckEditable(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{"UTF-8 text", []byte(userLine), nil},
		{"UTF-8 text with a byte order mark", append([]byte{0xEF, 0xBB, 0xBF}, userLine...), nil},
		{"empty file", nil, nil},
		{"UTF-16 little endian with a byte order mark", []byte{0xFF, 0xFE, 'S', 0, 'e', 0}, ErrNotUTF8},
		{"UTF-16 big endian with a byte order mark", []byte{0xFE, 0xFF, 0, 'S', 0, 'e'}, ErrNotUTF8},
		{"UTF-16 without a byte order mark", []byte{'S', 0, 'e', 0}, ErrNotUTF8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, CheckEditable(tt.data), tt.wantErr)
		})
	}
}

func TestMatchLineEndings(t *testing.T) {
	assert.Equal(t, "a\nb\n", MatchLineEndings("x\ny\n", "a\nb\n"), "an LF profile keeps LF")
	assert.Equal(t, "a\r\nb\r\n", MatchLineEndings("x\r\ny\r\n", "a\nb\n"), "a CRLF profile gets CRLF")
	assert.Equal(t, "a\nb\n", MatchLineEndings("", "a\nb\n"), "an empty profile gets LF")
}

func TestProfilePaths(t *testing.T) {
	home := t.TempDir()
	testutil.SetTestHomeDir(t, home)

	paths, err := ProfilePaths()
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		assert.Equal(t, []string{
			filepath.Join(home, "Documents", "PowerShell", profileFileName),
			filepath.Join(home, "Documents", "WindowsPowerShell", profileFileName),
		}, paths, "Windows has a profile for PowerShell 7 and one for Windows PowerShell")

		return
	}

	assert.Equal(t, []string{filepath.Join(home, ".config", "powershell", profileFileName)}, paths)
}

func TestInstallProfilePath(t *testing.T) {
	home := t.TempDir()
	testutil.SetTestHomeDir(t, home)

	if runtime.GOOS != "windows" {
		got, err := InstallProfilePath()
		require.NoError(t, err)

		assert.Equal(t, filepath.Join(home, ".config", "powershell", profileFileName), got)

		return
	}

	got, err := InstallProfilePath()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, "Documents", "WindowsPowerShell", profileFileName), got, "without a PowerShell 7 folder the Windows PowerShell profile is used")

	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "PowerShell"), 0o755))

	got, err = InstallProfilePath()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, "Documents", "PowerShell", profileFileName), got, "with a PowerShell 7 folder its profile is used")
}
