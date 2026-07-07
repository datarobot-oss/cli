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

package plugin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/dependencies"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/tools"
)

// ErrDepsDeclined is returned by CheckAndInstallDeps when the user explicitly
// declines to install missing plugin dependencies.
var ErrDepsDeclined = errors.New("plugin dependency installation declined by user")

// resolveInstalledPluginDir returns the directory where pluginName is installed,
// searching all managed plugin directories in priority order.
// Returns ("", nil) when the plugin is not found in any managed directory.
func resolveInstalledPluginDir(pluginName string) (string, error) {
	managedDirs, err := ManagedPluginsDirs()
	if err != nil {
		return "", err
	}

	for _, dir := range managedDirs {
		candidate := filepath.Join(dir, pluginName)

		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	return "", nil
}

// CheckAndInstallDeps reads the plugin's versions.yaml, checks prerequisites,
// and installs any missing or outdated tools.
//
// confirm is called only when deps need installing; returning false means "skip".
// out receives the prerequisite warning message.
//
// Returns nil when there are no deps, all are satisfied, or install succeeds.
// Returns ErrDepsDeclined when confirm returns false.
// Returns os.ErrNotExist (wrapped) errors silently — a missing versions.yaml is normal.
// Returns any other GetRequirementsFromDir error (bad YAML, permission denied, etc.) to the caller.
func CheckAndInstallDeps(pluginName string, confirm func() bool, out io.Writer) error {
	pluginDir, err := resolveInstalledPluginDir(pluginName)
	if err != nil {
		log.Debug("plugin dep check skipped: could not resolve managed plugins dirs", "error", err)

		return err
	}

	if pluginDir == "" {
		log.Debug("plugin dep check skipped: plugin not found in any managed dir", "plugin", pluginName)

		return nil
	}

	prereqs, _, err := tools.GetRequirementsFromDir(pluginDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Debug("plugin dep check skipped: no versions.yaml", "plugin", pluginName)

			return nil
		}

		return fmt.Errorf("reading plugin versions.yaml for %q: %w", pluginName, err)
	}

	if len(prereqs) == 0 {
		log.Debug("plugin dep check skipped: no prereqs declared", "plugin", pluginName)

		return nil
	}

	result := tools.CheckPrerequisiteList(prereqs)

	if len(result.MissingMsgs) == 0 && len(result.WrongVersionMsgs) == 0 {
		return nil
	}

	fmt.Fprint(out, tools.PrerequisitesMsg(result.MissingMsgs, result.WrongVersionMsgs))

	if !confirm() {
		return ErrDepsDeclined
	}

	toInstall := append(result.MissingTools, result.WrongVersionTools...)

	if _, err := dependencies.InstallPrerequisites(out, toInstall); err != nil {
		return fmt.Errorf("installing plugin dependencies for %q: %w", pluginName, err)
	}

	return nil
}
