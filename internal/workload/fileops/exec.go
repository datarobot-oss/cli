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

package fileops

import (
	"fmt"
	"os"
	"runtime"
)

const execBits = 0o111

// LocalExecutable reports a file mode's executable bit, or nil on a host that has none (Windows).
func LocalExecutable(mode os.FileMode) *bool {
	return localExecutable(mode, runtime.GOOS)
}

func localExecutable(mode os.FileMode, goos string) *bool {
	if goos == "windows" {
		return nil
	}

	exec := mode.Perm()&execBits != 0

	return &exec
}

// ApplyExecutable sets or clears path's executable bits to match exec. Unknown (nil) and Windows are no-ops.
func ApplyExecutable(path string, exec *bool) error {
	return applyExecutable(path, exec, runtime.GOOS)
}

func applyExecutable(path string, exec *bool, goos string) error {
	if exec == nil || goos == "windows" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	perm := info.Mode().Perm()
	want := perm &^ execBits

	// Execute follows read, so the umask that shaped the file's creation still applies.
	if *exec {
		want |= (perm & 0o444) >> 2
	}

	if want == perm {
		return nil
	}

	if err := os.Chmod(path, want); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}

	return nil
}

// ArchivePerm is the mode an uploaded zip entry carries for the executable bit.
func ArchivePerm(exec bool) os.FileMode {
	if exec {
		return 0o755
	}

	return 0o644
}
