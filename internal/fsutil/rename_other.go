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

//go:build !windows

package fsutil

// clearReadOnlyBeforeRename is a no-op on POSIX: os.Rename replaces a
// read-only destination without complaint, because the directory
// permissions, not the file's, govern the rename. See rename_windows.go for
// the Windows-only rationale.
func clearReadOnlyBeforeRename(_ string) error {
	return nil
}
