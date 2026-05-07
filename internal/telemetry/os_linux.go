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
	"strings"
)

// osVersion retrieves the Linux OS version by reading and parsing the
// /etc/os-release file. In containerized environments, this will
// typically return the underlying host OS version rather than the container's base image version. Returns an empty string if detection fails.
// example:
// ╰─❯ cat /etc/os-release content:
// NAME="Ubuntu"
// VERSION="22.04.3 LTS (Jammy Jellyfish)"
// ID=ubuntu
func osVersion() string {
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

// humanizeOS converts the raw OS name from runtime.GOOS into a more
// user-friendly format for telemetry. On Linux, we always return "Linux".
func humanizeOS(_ string) string {
	return "Linux"
}
