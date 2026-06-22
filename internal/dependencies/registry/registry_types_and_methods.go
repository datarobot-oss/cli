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
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// TAB is the indent prefix used in user-facing tip and failure messages.
const TAB = "  "

// Strategy is implemented by ManagerStrategy and FallbackStrategy.
// getStrategyTip returns the user-facing tip line for an install failure, or ""
// when no actionable suggestion is available.
// withVersion returns a copy of the strategy with {version} and {version_mm}
// placeholders in commands replaced by the given version string.
type Strategy interface {
	GetStrategyTip(goos string) string
	WithVersion(version string) Strategy
}

// ManagerStrategy provides install commands when a specific package/version manager
// is detected in the environment.
// DefaultVersion is substituted when the Prerequisite carries no MinimumVersion.
type ManagerStrategy struct {
	Manager        string
	Commands       []string
	DefaultVersion string
}

// FallbackStrategy is used when no manager-specific strategy matches.
// CommandsWindows overrides Commands on Windows when non-empty.
// DefaultVersion is substituted when the Prerequisite carries no MinimumVersion.
type FallbackStrategy struct {
	Commands        []string
	CommandsWindows []string
	DefaultVersion  string
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

func (ms ManagerStrategy) WithVersion(version string) Strategy {
	if version == "" {
		version = ms.DefaultVersion
	}

	ms.Commands = substituteCmds(ms.Commands, version)

	return ms
}

func (fs FallbackStrategy) WithVersion(version string) Strategy {
	if version == "" {
		version = fs.DefaultVersion
	}

	fs.Commands = substituteCmds(fs.Commands, version)
	fs.CommandsWindows = substituteCmds(fs.CommandsWindows, version)

	return fs
}

func (ms ManagerStrategy) GetStrategyTip(_ string) string {
	tipMsg := ms.Commands[0]

	if len(ms.Commands) > 1 {
		tipMsg = "\n" + TAB + TAB + strings.Join(ms.Commands, "\n"+TAB+TAB)
	}

	return fmt.Sprintf(TAB+"Tip: You have %s — try: %s", ms.Manager, tipMsg)
}

func (fs FallbackStrategy) GetStrategyTip(goos string) string {
	cmds := fs.Commands

	if goos == "windows" && len(fs.CommandsWindows) > 0 {
		cmds = fs.CommandsWindows
	}

	switch len(cmds) {
	case 0:
		if fs.URL != "" {
			return TAB + "See: " + fs.URL
		}

		return ""

	case 1:
		return TAB + "Try: " + cmds[0]

	default:
		return TAB + "Try:\n" + TAB + TAB + strings.Join(cmds, "\n"+TAB+TAB)
	}
}

// ToolInfo holds installation information for a dependency.
// Aliases contains lowercase alternate names (beyond the registry key and
// lowercased Name) that NormalizeToolName should resolve to this tool's key.
type ToolInfo struct {
	Name       string
	Aliases    []string
	Strategies []Strategy
}

// ToolRegistry maps tool keys (e.g. "python", "uv") to their installation info.
// Strategies are evaluated in order; the first matching one wins.
var ToolRegistry = map[string]ToolInfo{}

// buildToolNameMap lazily builds a lookup map from all display names and aliases to
// registry keys. Cached after the first call, after all init() functions have populated ToolRegistry.
var buildToolNameMap = sync.OnceValue(func() map[string]string {
	toolNameMap := make(map[string]string)

	for key, info := range ToolRegistry {
		toolNameMap[key] = key
		toolNameMap[strings.ToLower(info.Name)] = key

		for _, alias := range info.Aliases {
			toolNameMap[alias] = key
		}
	}

	return toolNameMap
})

// NormalizeToolName maps a dr CLI display name (e.g. "Taskfile task runner") to
// the corresponding ToolRegistry key (e.g. "task").
// Returns an empty string if the name is not recognized.
func NormalizeToolName(displayName string) string {
	return buildToolNameMap()[strings.ToLower(strings.TrimSpace(displayName))]
}

// detectEnvironment is the cached result of a single real environment probe.
var detectEnvironment = sync.OnceValue(func() map[string]bool {
	ctx := detectionCtx{
		lookPath: exec.LookPath,
		getenv:   os.Getenv,
		dirExists: func(p string) bool {
			info, err := os.Stat(p)

			return err == nil && info.IsDir()
		},
		goos: runtime.GOOS,
	}

	env := make(map[string]bool, len(knownManagers))

	for _, m := range knownManagers {
		env[m.Name] = m.present(ctx)
	}

	return env
})

// DetectEnvironment checks for available package/version managers.
// The returned map uses manager names as keys (e.g. "brew", "pyenv").
// Result is cached after the first call.
func DetectEnvironment() map[string]bool {
	return detectEnvironment()
}

// SelectInstallStrategy returns the first matching Strategy for toolKey.
// ManagerStrategy entries whose Manager equals failedMgr are skipped.
// Returns ManagerStrategy when a detected manager matches, FallbackStrategy
// as last resort, or nil when toolKey is unknown.
func SelectInstallStrategy(toolKey, failedMgr string, env map[string]bool) Strategy {
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
