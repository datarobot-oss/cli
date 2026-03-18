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

package shared

import (
	"fmt"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
)

// NormalizeRegistryURL ensures the URL ends with index.json
func NormalizeRegistryURL(url string) string {
	if len(url) > 0 && url[len(url)-1] == '/' {
		return url + "index.json"
	}

	if len(url) > 5 && url[len(url)-5:] != ".json" {
		return url + "/index.json"
	}

	return url
}

// RunPluginUpdate performs the backup → install → validate → rollback cycle
// for upgrading a managed plugin. It prints styled status messages and returns
// true only when the update succeeds and validation passes.
func RunPluginUpdate(pluginName, fromVersion string, entry plugin.RegistryPlugin, version plugin.RegistryVersion, baseURL string) bool {
	fmt.Printf("Updating %s from %s to %s...\n", pluginName, fromVersion, version.Version)

	backupPath, err := plugin.BackupPlugin(pluginName)
	if err != nil {
		fmt.Println(tui.ErrorStyle.Render(fmt.Sprintf("✗ Failed to backup %s: %v", pluginName, err)))

		return false
	}
	defer plugin.CleanupBackup(backupPath)

	if err := plugin.InstallPlugin(entry, version, baseURL); err != nil {
		fmt.Println(tui.ErrorStyle.Render(fmt.Sprintf("✗ Failed to update %s: %v", pluginName, err)))
		rollbackPlugin(pluginName, backupPath)

		return false
	}

	if err := plugin.ValidatePlugin(pluginName); err != nil {
		fmt.Println(tui.ErrorStyle.Render(fmt.Sprintf("✗ Plugin validation failed: %v", err)))
		rollbackPlugin(pluginName, backupPath)

		return false
	}

	fmt.Println(tui.SuccessStyle.Render("✓ Updated " + pluginName + " to " + version.Version))

	return true
}

// rollbackPlugin attempts to restore a plugin from its backup, printing the outcome.
func rollbackPlugin(pluginName, backupPath string) {
	fmt.Println("Rolling back to previous version...")

	if restoreErr := plugin.RestorePlugin(pluginName, backupPath); restoreErr != nil {
		fmt.Println(tui.ErrorStyle.Render(fmt.Sprintf("✗ Failed to restore backup: %v", restoreErr)))
	} else {
		fmt.Println(tui.SuccessStyle.Render("✓ Restored previous version"))
	}
}
