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
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	osVersionOnce  sync.Once
	osVersionCache string
)

// detectOSVersion returns the host OS version string. The result is computed
// once and cached for the lifetime of the process. Returns empty string if
// detection fails.
func detectOSVersion() string {
	osVersionOnce.Do(func() {
		osVersionCache = readOSVersion()
	})

	return osVersionCache
}

func readOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return darwinVersion()
	case "linux":
		return linuxVersion()
	case "windows":
		return windowsVersion()
	default:
		return ""
	}
}

func linuxVersion() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "VERSION_ID=") {
			val := strings.TrimPrefix(line, "VERSION_ID=")
			val = strings.Trim(val, `"`)

			return val
		}
	}

	return ""
}

func windowsVersion() string {
	out, err := exec.Command("cmd", "/c", "ver").Output()
	if err != nil {
		return ""
	}

	// "ver" output: "Microsoft Windows [Version 10.0.22621.1234]"
	s := strings.TrimSpace(string(out))
	start := strings.Index(s, "[Version ")

	if start == -1 {
		return ""
	}

	s = s[start+len("[Version "):]
	end := strings.Index(s, "]")

	if end == -1 {
		return ""
	}

	return s[:end]
}

// humanizeOS maps runtime.GOOS values to platform names users will recognize.
// This should help with Amplitude event analysis.
func humanizeOS(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	default:
		// I hope we never get here but if we do, just return the raw GOOS value
		return goos
	}
}
