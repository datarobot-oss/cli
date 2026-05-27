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
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/plugin"
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
// for upgrading a managed plugin. It returns nil only when the update succeeds
// and validation passes.
func RunPluginUpdate(pluginName, _ string, entry plugin.RegistryPlugin, version plugin.RegistryVersion, baseURL string) error {
	backupPath, err := plugin.BackupPlugin(pluginName)
	if err != nil {
		return fmt.Errorf("backup %s: %w", pluginName, err)
	}

	defer plugin.CleanupBackup(backupPath)

	if err := plugin.InstallPlugin(entry, version, baseURL); err != nil {
		rollbackErr := rollbackPlugin(pluginName, backupPath)
		if rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("install %s: %w", pluginName, err),
				rollbackErr,
			)
		}

		return fmt.Errorf("install %s: %w", pluginName, err)
	}

	if err := plugin.ValidatePlugin(pluginName); err != nil {
		rollbackErr := rollbackPlugin(pluginName, backupPath)
		if rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("validate %s: %w", pluginName, err),
				rollbackErr,
			)
		}

		return fmt.Errorf("validate %s: %w", pluginName, err)
	}

	return nil
}

// rollbackPlugin attempts to restore a plugin from its backup.
func rollbackPlugin(pluginName, backupPath string) error {
	if restoreErr := plugin.RestorePlugin(pluginName, backupPath); restoreErr != nil {
		return fmt.Errorf("restore backup for %s: %w", pluginName, restoreErr)
	}

	return nil
}
