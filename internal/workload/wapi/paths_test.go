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

package wapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExists_Missing(t *testing.T) {
	tmp := t.TempDir()

	assert.False(t, Exists(tmp))
}

func TestExists_PresentDir(t *testing.T) {
	tmp := t.TempDir()

	err := os.Mkdir(filepath.Join(tmp, DirName), 0o755)
	require.NoError(t, err)

	assert.True(t, Exists(tmp))
}

func TestExists_IsFileNotDir(t *testing.T) {
	tmp := t.TempDir()

	err := os.WriteFile(filepath.Join(tmp, DirName), []byte("oops"), 0o644)
	require.NoError(t, err)

	assert.False(t, Exists(tmp))
}
