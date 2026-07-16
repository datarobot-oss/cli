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

package testutil

import (
	"testing"

	"github.com/adrg/xdg"
)

// SetTestHomeDir sets the home directory for tests to work cross-platform.
// Both HOME (Unix) and USERPROFILE (Windows) are set so os.UserHomeDir() works everywhere.
// XDG_CONFIG_HOME is unset to ensure tests use the HOME/.config fallback path.
func SetTestHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	SetXDGEnv(t, "XDG_CONFIG_HOME", "")
}

// SetXDGEnv sets an XDG_* environment variable for the duration of the test
// and reloads github.com/adrg/xdg's cached base directories so the change
// takes effect immediately. Without this, xdg.ConfigHome/xdg.StateHome/
// xdg.ConfigDirs would keep reflecting whatever the environment looked like
// at process start (or the last Reload), ignoring the env var set here.
func SetXDGEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
	xdg.Reload()
}
