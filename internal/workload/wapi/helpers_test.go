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

	"github.com/stretchr/testify/require"
)

// initWapiDir creates an empty .wapi/ directory inside projectDir so that
// Load* / Save* / AppendHistory operations bypass their ErrNotInitialized
// short-circuit. Used by config, manifest, and history tests that only care
// about the read/write behaviour, not the Exists precondition.
func initWapiDir(t *testing.T, projectDir string) {
	t.Helper()

	err := os.MkdirAll(filepath.Join(projectDir, DirName), 0o755)
	require.NoError(t, err)
}
