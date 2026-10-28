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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalExecutable(t *testing.T) {
	for _, tc := range []struct {
		mode os.FileMode
		want bool
	}{
		{0o755, true},
		{0o744, true},
		{0o701, true},
		{0o644, false},
		{0o600, false},
	} {
		got := localExecutable(tc.mode, "linux")
		require.NotNil(t, got)
		assert.Equal(t, tc.want, *got, "%o", tc.mode)
	}

	assert.Nil(t, localExecutable(0o755, "windows"), "Windows has no bit to report")
}

func TestApplyExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable bit")
	}

	yes, no := true, false
	path := filepath.Join(t.TempDir(), "run.sh")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	require.NoError(t, os.Chmod(path, 0o640))

	perm := func() os.FileMode {
		info, err := os.Stat(path)
		require.NoError(t, err)

		return info.Mode().Perm()
	}

	require.NoError(t, applyExecutable(path, &yes, "linux"))
	assert.Equal(t, os.FileMode(0o750), perm(), "execute follows read, so others stay out")

	require.NoError(t, applyExecutable(path, nil, "linux"))
	assert.Equal(t, os.FileMode(0o750), perm(), "unknown leaves the file alone")

	require.NoError(t, applyExecutable(path, &no, "windows"))
	assert.Equal(t, os.FileMode(0o750), perm(), "never chmod on Windows")

	require.NoError(t, applyExecutable(path, &no, "linux"))
	assert.Equal(t, os.FileMode(0o640), perm())
}

func TestArchivePerm(t *testing.T) {
	assert.Equal(t, os.FileMode(0o755), ArchivePerm(true))
	assert.Equal(t, os.FileMode(0o644), ArchivePerm(false))
}
