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

package telemetry

import (
	"os/exec"
	"strings"
)

// osLanguage returns the user's language tag on macOS.
// LANG is authoritative when set (e.g. in a terminal session). When it is
// absent — common for GUI-launched processes — we fall back to reading the
// system locale via `defaults read NSGlobalDomain AppleLocale`, which returns
// a value like "en_US". Returns empty string if all sources fail.
func osLanguage() string {
	if lang := langFromEnv(); lang != "" {
		return lang
	}

	out, err := exec.Command("defaults", "read", "NSGlobalDomain", "AppleLocale").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
