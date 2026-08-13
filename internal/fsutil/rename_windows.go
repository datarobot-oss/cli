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

//go:build windows

package fsutil

import (
	"fmt"
	"syscall"
)

// clearReadOnlyBeforeRename clears the FILE_ATTRIBUTE_READONLY attribute on
// the destination of an upcoming os.Rename. os.Rename on Windows fails with
// "Access is denied" when the destination is read-only (golang/go#38287,
// unfixed through Go 1.26), so an atomic write that renames a temp file over
// a read-only target would fail where its POSIX counterpart succeeds. This
// mirrors what os.Remove does: read the attributes, clear the read-only bit,
// and let the caller proceed. The attribute is restored by the chmod that
// follows the rename in AtomicWriteFile, because os.Stat reports the
// read-only file as 0o444 and targetMode returns that.
func clearReadOnlyBeforeRename(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("clearReadOnly %s: %w", path, err)
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		// The file may not exist yet (the rename will create it), or the
		// volume does not support attributes; either way there is nothing
		// to clear.
		return nil
	}

	if attrs&syscall.FILE_ATTRIBUTE_READONLY == 0 {
		return nil
	}

	return syscall.SetFileAttributes(p, attrs&^syscall.FILE_ATTRIBUTE_READONLY)
}
