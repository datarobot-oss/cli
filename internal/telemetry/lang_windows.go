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
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var procGetUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")

// osLanguage returns the user's locale name on Windows by calling
// GetUserDefaultLocaleName from kernel32.dll. Returns a BCP 47 tag like
// "en-US". Returns empty string if the call fails.
func osLanguage() string {
	// lpLocaleName buffer: LOCALE_NAME_MAX_LENGTH is 85 chars
	const maxLen = 85

	buf := make([]uint16, maxLen)

	ret, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(maxLen),
	)

	if ret == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}
