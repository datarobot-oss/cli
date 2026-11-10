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

package auth

import (
	"fmt"
	"io"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	"github.com/spf13/cobra"
)

// announceCredentialsSaved writes the single save notice shown after a
// successful `dr auth login` or `dr auth set-url` persistence. The notice
// names whose credentials were saved -- the root-level credentials when the
// profile selection is omitted, otherwise the normalized profile name,
// including an explicit "default" (see config.ProfileDescription) -- and the
// drconfig.yaml that actually holds them, so users can see where the write
// landed.
//
// Callers emit it exactly once per invocation, only after the persistence it
// describes has succeeded, and on the error stream: --output-format json
// keeps stdout reserved for the command's machine-readable output. The
// notice never includes credential values.
func announceCredentialsSaved(w io.Writer, profileName, configPath string) {
	fmt.Fprintf(w, "Saved %s to %s\n", config.ProfileDescription(profileName), configPath)
}

// WriteConfigFileAndAnnounce persists the allowlisted config keys and, only
// after the write succeeded, emits the single save notice naming the saved
// profile and the actual config file. Both `auth login` and `auth set-url`
// end through here so a successful save prints one notice, never two
// overlapping ones.
//
// A write failure is returned (and logged by WriteConfigFileSilent); the
// caller reports it and skips authentication, so a failed write is never
// followed by a success claim or a notice.
func WriteConfigFileAndAnnounce(cmd *cobra.Command) error {
	if err := WriteConfigFileSilent(); err != nil {
		return err
	}

	AnnounceCredentialsSavedForCommand(cmd)

	return nil
}

// AnnounceCredentialsSavedForCommand resolves the just-saved profile from the
// active selection and the config file viper actually used, then writes the
// save notice to the command's error stream.
//
// A path-resolution failure is logged, not returned: the persistence this
// notice would describe has already succeeded, and turning a completed save
// into a command error would misreport the outcome.
func AnnounceCredentialsSavedForCommand(cmd *cobra.Command) {
	configPath, err := config.ConfigFilePath()
	if err != nil {
		log.Error("Failed to resolve the config file path for the save notice.", "error", err)

		return
	}

	announceCredentialsSaved(cmd.ErrOrStderr(), config.ActiveProfile(), configPath)
}
