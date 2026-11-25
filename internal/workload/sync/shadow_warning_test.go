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

package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// wantShadowWarning pins the exact text the ignore matcher emits when both
// ignore filenames are present. It is transcribed rather than produced by
// the same Sprintf, so a wording change is a deliberate act.
const wantShadowWarning = "Both .drignore and .wapiignore are present. .drignore is the one in effect, " +
	"and the patterns in .wapiignore are not applied. Merge them into .drignore and delete .wapiignore."

// fsIsCaseInsensitive reports whether dir lives on a case-insensitive
// filesystem, using a throwaway probe pair that is removed before returning
// so the caller's fixture is untouched.
func fsIsCaseInsensitive(t *testing.T, dir string) bool {
	t.Helper()

	probeDir := filepath.Join(dir, "case-probe")
	require.NoError(t, os.MkdirAll(probeDir, 0o755))

	lower := filepath.Join(probeDir, "probe.txt")
	require.NoError(t, os.WriteFile(lower, []byte("x"), 0o644))

	_, err := os.Stat(filepath.Join(probeDir, "PROBE.TXT"))

	require.NoError(t, os.RemoveAll(probeDir))

	return err == nil
}
