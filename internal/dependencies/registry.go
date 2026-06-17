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

package dependencies

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Strategy is implemented by ManagerStrategy and FallbackStrategy.
// getStrategyTip returns the user-facing tip line for an install failure, or ""
// when no actionable suggestion is available.
// withVersion returns a copy of the strategy with {version} and {version_mm}
// placeholders in commands replaced by the given version string.
type Strategy interface {
	getStrategyTip(goos string) string
	withVersion(version string) Strategy
}

// ManagerStrategy provides install commands when a specific package/version manager
// is detected in the environment.
type ManagerStrategy struct {
	Manager  string
	Commands []string
}

// FallbackStrategy is used when no manager-specific strategy matches.
// CommandsWindows overrides Commands on Windows when non-empty.
type FallbackStrategy struct {
	Commands        []string
	CommandsWindows []string
	Message         string
	URL             string
}

// majorMinorVersion extracts the major.minor portion from a semver string.
// "3.9.6" → "3.9", "24.0.0" → "24.0", "" → "".
func majorMinorVersion(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}

	return parts[0] + "." + parts[1]
}

// substituteCmds replaces {version_mm} and {version} placeholders in each command.
// {version_mm} is substituted first to avoid a partial match against {version}.
func substituteCmds(cmds []string, version string) []string {
	if len(cmds) == 0 {
		return cmds
	}

	out := make([]string, len(cmds))
	mm := majorMinorVersion(version)

	for i, c := range cmds {
		c = strings.ReplaceAll(c, "{version_mm}", mm)
		out[i] = strings.ReplaceAll(c, "{version}", version)
	}

	return out
}

func (ms ManagerStrategy) withVersion(version string) Strategy {
	if version == "" {
		return ms
	}

	ms.Commands = substituteCmds(ms.Commands, version)

	return ms
}

func (fs FallbackStrategy) withVersion(version string) Strategy {
	if version == "" {
		return fs
	}

	fs.Commands = substituteCmds(fs.Commands, version)
	fs.CommandsWindows = substituteCmds(fs.CommandsWindows, version)

	return fs
}

func (ms ManagerStrategy) getStrategyTip(_ string) string {
	tipMsg := ms.Commands[0]

	if len(ms.Commands) > 1 {
		tipMsg = "\n\t" + strings.Join(ms.Commands, "\n\t")
	}

	return fmt.Sprintf("  Tip: You have %s — try: %s", ms.Manager, tipMsg)
}

func (fs FallbackStrategy) getStrategyTip(goos string) string {
	cmds := fs.Commands

	if goos == "windows" && len(fs.CommandsWindows) > 0 {
		cmds = fs.CommandsWindows
	}

	switch len(cmds) {
	case 0:
		if fs.URL != "" {
			return "  See: " + fs.URL
		}

		return ""

	case 1:
		return "  Try: " + cmds[0]

	default:
		return "  Try:\n\t" + strings.Join(cmds, "\n\t")
	}
}

// ToolInfo holds installation information for a dependency.
type ToolInfo struct {
	Name       string
	Strategies []Strategy
}

// ToolRegistry maps tool keys (e.g. "python", "uv") to their installation info.
// Strategies are evaluated in order; the first matching one wins.
var ToolRegistry = map[string]ToolInfo{
	"python": {
		Name: "Python",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "pyenv", Commands: []string{"pyenv install {version}", "pyenv global {version}"}},
			ManagerStrategy{Manager: "asdf", Commands: []string{"asdf install python {version}", "asdf global python {version}"}},
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install python@{version_mm}"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install Python.Python.{version_mm}"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install python --version={version}"}},
			FallbackStrategy{
				Message: "Install pyenv (recommended for managing Python versions):",
				Commands: []string{
					"curl https://pyenv.run | bash",
					"# Restart terminal, then:",
					"pyenv install {version}",
					"pyenv global {version}",
				},
				CommandsWindows: []string{
					"# Install pyenv-win via PowerShell:",
					`Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/pyenv-win/pyenv-win/master/pyenv-win/install-pyenv-win.ps1" -OutFile "./install-pyenv-win.ps1"; &"./install-pyenv-win.ps1"`,
					"# Restart terminal, then:",
					"pyenv install {version}",
					"pyenv global {version}",
				},
				URL: "https://www.python.org/downloads/",
			},
		},
	},
	// uv: pyenv strategy first — if the user manages Python via pyenv, pip install
	// is the most natural path; brew/asdf/curl follow in priority order.
	"uv": {
		Name: "uv",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "pyenv", Commands: []string{"pip install uv"}},
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install uv"}},
			ManagerStrategy{Manager: "asdf", Commands: []string{
				"asdf plugin add uv https://github.com/asdf-community/asdf-uv.git",
				"asdf install uv latest",
				"asdf global uv latest",
			}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install astral-sh.uv"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install uv"}},
			FallbackStrategy{
				Commands:        []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"},
				CommandsWindows: []string{`powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`},
				URL:             "https://docs.astral.sh/uv/getting-started/installation/",
			},
		},
	},
	"node": {
		Name: "Node.js",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "nvm", Commands: []string{"nvm install {version}", "nvm use {version}"}},
			ManagerStrategy{Manager: "fnm", Commands: []string{"fnm install {version}", "fnm use {version}"}},
			ManagerStrategy{Manager: "asdf", Commands: []string{"asdf install nodejs {version}", "asdf global nodejs {version}"}},
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install node"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install OpenJS.NodeJS"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install nodejs"}},
			FallbackStrategy{
				Message: "Install a version manager (recommended):",
				Commands: []string{
					"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash",
					"# Restart terminal, then:",
					"nvm install {version}",
					"nvm use {version}",
				},
				URL: "https://nodejs.org/",
			},
		},
	},
	"pulumi": {
		Name: "Pulumi",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install pulumi"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install Pulumi.Pulumi"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install pulumi"}},
			FallbackStrategy{
				Commands:        []string{"curl -fsSL https://get.pulumi.com | sh"},
				CommandsWindows: []string{"iwr -useb https://get.pulumi.com/install.ps1 | iex"},
				URL:             "https://www.pulumi.com/docs/install/",
			},
		},
	},
	"task": {
		Name: "Task",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install go-task"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install Task.Task"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install go-task"}},
			ManagerStrategy{Manager: "scoop", Commands: []string{"scoop install task"}},
			FallbackStrategy{
				Commands:        []string{`sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d`},
				CommandsWindows: []string{"# Download the executable from the releases page:"},
				URL:             "https://taskfile.dev/installation/",
			},
		},
	},
	"git": {
		Name: "Git",
		Strategies: []Strategy{
			ManagerStrategy{Manager: "brew", Commands: []string{"brew install git"}},
			ManagerStrategy{Manager: "winget", Commands: []string{"winget install Git.Git"}},
			ManagerStrategy{Manager: "choco", Commands: []string{"choco install git"}},
			FallbackStrategy{
				URL: "https://git-scm.com/downloads",
			},
		},
	},
}

// knownManagers lists manager names checked by extractFailedManager.
var knownManagers = []string{"brew", "pyenv", "asdf", "nvm", "fnm", "winget", "choco", "scoop"}

// toolNameMap maps lowercase dr CLI display names to ToolRegistry keys.
var toolNameMap = map[string]string{
	// Canonical keys
	"python":                             "python",
	"uv":                                 "uv",
	"node":                               "node",
	"node.js":                            "node",
	"nodejs":                             "node",
	"pulumi":                             "pulumi",
	"pulumi infrastructure as code tool": "pulumi",
	"task":                               "task",
	"taskfile task runner":               "task",
	"git":                                "git",
	"git source control management tool": "git",
	// Python aliases
	"py":       "python",
	"py3":      "python",
	"python3":  "python",
	"python@3": "python",
}

// NormalizeToolName maps a dr CLI display name (e.g. "Taskfile task runner") to
// the corresponding ToolRegistry key (e.g. "task").
// Returns an empty string if the name is not recognized.
func NormalizeToolName(displayName string) string {
	return toolNameMap[strings.ToLower(strings.TrimSpace(displayName))]
}

// DetectEnvironment checks for available package/version managers and platform flags.
// The returned map uses manager names as keys (e.g. "brew", "pyenv") plus "is_windows".
func DetectEnvironment() map[string]bool {
	return detectEnvironment(
		exec.LookPath,
		os.Getenv,
		func(p string) bool {
			info, err := os.Stat(p)

			return err == nil && info.IsDir()
		},
		runtime.GOOS,
	)
}

func detectEnvironment(
	lookPath func(string) (string, error),
	getenv func(string) string,
	dirExists func(string) bool,
	goos string,
) map[string]bool {
	isWindows := goos == "windows"

	present := func(name string) bool {
		_, err := lookPath(name)

		return err == nil
	}

	nvmDir := getenv("NVM_DIR")

	if nvmDir == "" {
		home := getenv("HOME")
		nvmDir = filepath.Join(home, ".nvm")
	}

	nvmPresent := !isWindows && dirExists(nvmDir)

	return map[string]bool{
		"pyenv":      present("pyenv"),
		"nvm":        nvmPresent,
		"fnm":        present("fnm"),
		"asdf":       present("asdf"),
		"brew":       present("brew") && !isWindows,
		"winget":     present("winget") && isWindows,
		"choco":      present("choco") && isWindows,
		"scoop":      present("scoop") && isWindows,
		"is_windows": isWindows,
	}
}

// selectInstallStrategy returns the first matching Strategy for toolKey.
// ManagerStrategy entries whose Manager equals failedMgr are skipped.
// Returns ManagerStrategy when a detected manager matches, FallbackStrategy
// as last resort, or nil when toolKey is unknown.
func selectInstallStrategy(toolKey, failedMgr string, env map[string]bool) Strategy {
	toolKey = NormalizeToolName(toolKey)

	tool, ok := ToolRegistry[toolKey]
	if !ok {
		return nil
	}

	for _, s := range tool.Strategies {
		switch strategy := s.(type) {
		case ManagerStrategy:
			// Do not provide a tip for the detected manager if it was involved in the failed install attempt, since that may be why the strategy failed;
			// Instead, continue checking other strategies.
			if strategy.Manager != failedMgr && env[strategy.Manager] {
				return strategy
			}

		case FallbackStrategy:
			return strategy
		}
	}

	return nil
}
