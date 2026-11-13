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

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/datarobot/cli/cmd/self/completion/powershell"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/spf13/cobra"
)

// powerShellInstaller writes the completion blocks into the PowerShell profile. command is what the
// blocks run to print the completion script.
type powerShellInstaller struct {
	path    string
	command string
}

func newPowerShellInstaller(command string) (powerShellInstaller, error) {
	profilePath, err := powershell.InstallProfilePath()
	if err != nil {
		return powerShellInstaller{}, err
	}

	return powerShellInstaller{path: profilePath, command: command}, nil
}

func (p powerShellInstaller) Path() string {
	return p.path
}

// Installed reports whether the profile holds every block and no PowerShell profile runs the legacy
// command. A profile written by an earlier release is not installed, so an install migrates it.
func (p powerShellInstaller) Installed() bool {
	paths, err := powershell.ProfilePaths()
	if err != nil {
		return false
	}

	return p.installedAcross(paths)
}

func (p powerShellInstaller) installedAcross(paths []string) bool {
	data, err := os.ReadFile(p.path)
	if err != nil || powershell.MissingBlocks(string(data), p.command) != "" {
		return false
	}

	for _, path := range paths {
		if holdsLegacyBlock(path, p.command) {
			return false
		}
	}

	return true
}

// holdsLegacyBlock reports whether the profile has a block that runs the legacy command.
func holdsLegacyBlock(profilePath, command string) bool {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return false
	}

	_, legacy := powershell.MigrateLegacy(string(data), command)

	return legacy
}

func (p powerShellInstaller) Install(*cobra.Command) error {
	shellExe := powerShellExeForProfile()

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("Failed to create PowerShell profile directory: %w", err)
	}

	changed, err := updatePowerShellProfile(p.path, p.command)
	if err != nil {
		return err
	}

	migratedOthers, err := migrateOtherPowerShellProfiles(p.path, p.command)
	if err != nil {
		return err
	}

	if changed || migratedOthers {
		warnIfExecutionPolicyRestricted(shellExe)
	}

	return nil
}

// readEditableProfile reads a profile and refuses one that byte-level edits would corrupt.
func readEditableProfile(profilePath string) ([]byte, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read PowerShell profile: %w", err)
	}

	if err := powershell.CheckEditable(data); err != nil {
		return nil, fmt.Errorf("Refusing to edit %s: %w. Save it as UTF-8 and run the command again.", profilePath, err)
	}

	return data, nil
}

// updatePowerShellProfile adds the completion blocks the profile lacks and reports whether it
// changed the file. A profile written by an earlier release holds the legacy command, so it is
// rewritten in place. Any other profile only gets the missing blocks appended.
func updatePowerShellProfile(profilePath, command string) (bool, error) {
	// Read existing profile content to detect which blocks are already installed.
	var existingContent string

	if fsutil.FileExists(profilePath) {
		data, err := readEditableProfile(profilePath)
		if err != nil {
			return false, err
		}

		existingContent = string(data)
	}

	updatedContent, migrated := powershell.MigrateLegacy(existingContent, command)
	missingScript := powershell.MatchLineEndings(updatedContent, powershell.MissingBlocks(updatedContent, command))

	switch {
	case migrated:
		return true, writePowerShellProfile(profilePath, updatedContent+missingScript)
	case missingScript == "":
		return false, nil // Already fully installed
	default:
		return true, appendToPowerShellProfile(profilePath, missingScript)
	}
}

// migrateOtherPowerShellProfiles replaces the legacy command in the profiles of the other
// PowerShell, which keep printing an error in every session until they are migrated too. It
// leaves a profile that holds no legacy block alone.
func migrateOtherPowerShellProfiles(installedPath, command string) (bool, error) {
	paths, err := powershell.ProfilePaths()
	if err != nil {
		return false, err
	}

	return migrateProfiles(paths, installedPath, command)
}

func migrateProfiles(paths []string, installedPath, command string) (bool, error) {
	var changed bool

	for _, path := range paths {
		if path == installedPath || !fsutil.FileExists(path) {
			continue
		}

		data, err := readEditableProfile(path)
		if errors.Is(err, powershell.ErrNotUTF8) {
			fmt.Printf("%s %s\n", warnStyle.Render("Warning:"), err)

			continue
		}

		if err != nil {
			return changed, err
		}

		migrated, ok := powershell.MigrateLegacy(string(data), command)
		if !ok {
			continue
		}

		if err := writePowerShellProfile(path, migrated); err != nil {
			return changed, err
		}

		fmt.Printf("%s Completion command updated in: %s\n", successStyle.Render("✓"), path)

		changed = true
	}

	return changed, nil
}

// appendToPowerShellProfile appends content to the profile and reports a failed close.
func appendToPowerShellProfile(profilePath, content string) (err error) {
	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to open PowerShell profile: %w", err)
	}

	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("Failed to close PowerShell profile: %w", closeErr)
		}
	}()

	if _, err = f.WriteString(content); err != nil {
		return fmt.Errorf("Failed to write to PowerShell profile: %w", err)
	}

	return nil
}

// writePowerShellProfile replaces the profile through a temporary file, so a failed write leaves
// the user's profile as it was. The write keeps the file's mode and follows a symlinked profile.
func writePowerShellProfile(profilePath, content string) error {
	if err := fsutil.AtomicWriteFile(profilePath, []byte(content)); err != nil {
		return fmt.Errorf("Failed to write to PowerShell profile: %w", err)
	}

	return nil
}

// powerShellExeForProfile returns the PowerShell binary name that matches the
// profile path chosen by powershell.InstallProfilePath. PS Core (pwsh) and Windows
// PowerShell (powershell) maintain separate execution policy settings, so the
// warning check must query the same shell that will actually load the profile.
func powerShellExeForProfile() string {
	if runtime.GOOS != "windows" {
		return "powershell"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "powershell"
	}

	if fsutil.DirExists(filepath.Join(homeDir, "Documents", "PowerShell")) {
		return "pwsh"
	}

	return "powershell"
}
