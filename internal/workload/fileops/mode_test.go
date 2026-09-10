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

func TestApplyMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	t.Run("chmods for a non-zero mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "f")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

		require.NoError(t, ApplyMode(path, 0o755))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("no-ops for a zero mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "f")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

		require.NoError(t, ApplyMode(path, 0))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
}
