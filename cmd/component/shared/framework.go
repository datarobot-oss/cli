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
	"os"
	"path/filepath"
)

// FrameworkPath is set by the --framework-path persistent flag on the component parent command.
// Child commands read it via GetFrameworkPath() rather than directly so the default is applied.
var FrameworkPath string

// GetFrameworkPath returns the configured framework path, applying the default
// (~/.datarobot/app-framework/) if the flag was not set.
func GetFrameworkPath() string {
	if FrameworkPath != "" {
		return FrameworkPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".datarobot", "app-framework")
	}

	return filepath.Join(home, ".datarobot", "app-framework")
}
