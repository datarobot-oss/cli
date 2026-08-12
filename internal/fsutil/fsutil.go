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

package fsutil

import "os"

// FileExists checks if a given file exists. Every stat error is false, not
// just ErrNotExist: ENOTDIR and EACCES return no FileInfo to inspect.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// DirExists checks if a given directory exists, with the same error handling
// as FileExists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// PathExists checks if a given path exists (either file or directory).
func PathExists(path string) bool {
	_, err := os.Stat(path)

	return !os.IsNotExist(err)
}
