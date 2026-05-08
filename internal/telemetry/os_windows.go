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
	"fmt"

	"golang.org/x/sys/windows"
)

// osVersion retrieves the Windows OS version via RtlGetVersion (ntdll.dll).
// Returns an empty string if detection fails.
// example output: "10.0.22621"
func osVersion() string {
	info := windows.RtlGetVersion()

	return fmt.Sprintf("%d.%d.%d", info.MajorVersion, info.MinorVersion, info.BuildNumber)
}

// humanizeOS converts the raw OS name from runtime.GOOS into a more
// user-friendly format for telemetry. On Windows, we always return "Windows".
func humanizeOS(_ string) string {
	return "Windows"
}
