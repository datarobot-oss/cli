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

import "syscall"

// returns the MacOS marketing version using "kern.osproductversion" sysctl.
// Returns an empty string if detection fails.
// example:
// ╰─❯ sysctl kern.osproductversion
// kern.osproductversion: 15.7.5
func osVersion() string {
	// Note: runtime doesn't provide OSVERSION.
	// Note: Originally used "sw_vers -productVersion"
	// ref: https://ss64.com/mac/sw_vers.html
	// but that requires spawning a subprocess and will
	// anyway invoke sysctl under the hood.
	ver, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}

	return ver
}

// humanizeOS converts the raw OS name from runtime.GOOS into a more
// user-friendly format for telemetry. On Darwin, we always return "macOS".
func humanizeOS(_ string) string {
	return "macOS"
}
