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

package registry

import (
	"os"
	"path/filepath"
)

// detectionCtx bundles the injectable dependencies used by managerDef.present.
type detectionCtx struct {
	lookPath  func(string) (string, error)
	getenv    func(string) string
	dirExists func(string) bool
	goos      string
}

func (ctx detectionCtx) hasCommand(name string) bool {
	_, err := ctx.lookPath(name)

	return err == nil
}

// managerDef describes a package/version manager and how to detect its presence.
// present uses ctx.hasCommand for most managers; special cases like nvm override it.
type managerDef struct {
	Name    string
	present func(ctx detectionCtx) bool
}

var knownManagers = []managerDef{
	{
		Name:    "brew",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("brew") && ctx.goos != "windows" },
	},
	{
		Name:    "pyenv",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("pyenv") },
	},
	{
		Name:    "asdf",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("asdf") },
	},
	{
		Name: "nvm",
		present: func(ctx detectionCtx) bool {
			if ctx.goos == "windows" {
				return false
			}

			nvmDir := ctx.getenv("NVM_DIR")

			if nvmDir == "" {
				if home, err := os.UserHomeDir(); err == nil {
					nvmDir = filepath.Join(home, ".nvm")
				}
			}

			return ctx.dirExists(nvmDir)
		},
	},
	{
		Name:    "fnm",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("fnm") },
	},
	{
		Name:    "winget",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("winget") && ctx.goos == "windows" },
	},
	{
		Name:    "choco",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("choco") && ctx.goos == "windows" },
	},
	{
		Name:    "scoop",
		present: func(ctx detectionCtx) bool { return ctx.hasCommand("scoop") && ctx.goos == "windows" },
	},
}

// KnownManagers lists package/version manager names used by extractFailedManager
// to identify which manager was referenced in a failed install command.
var KnownManagers []string

func init() {
	KnownManagers = make([]string, len(knownManagers))

	for i, m := range knownManagers {
		KnownManagers[i] = m.Name
	}
}
