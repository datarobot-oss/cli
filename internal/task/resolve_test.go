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

package task

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func setRunStack(t *testing.T, assignment string) {
	t.Helper()

	key, value, _ := strings.Cut(assignment, "=")
	t.Setenv(key, value)
}

func TestGuardRunStackAllowsFirstInvocation(t *testing.T) {
	taskfile := filepath.Join(t.TempDir(), GeneratedTaskfileName)

	assignment, err := GuardRunStack(taskfile, []string{"lint"})
	require.NoError(t, err)
	require.Equal(t, RunStackEnv+"="+taskfile+"\x1flint", assignment)
}

func TestGuardRunStackRejectsSelfReentry(t *testing.T) {
	taskfile := filepath.Join(t.TempDir(), GeneratedTaskfileName)

	assignment, err := GuardRunStack(taskfile, []string{"lint"})
	require.NoError(t, err)
	setRunStack(t, assignment)

	_, err = GuardRunStack(taskfile, []string{"lint"})
	require.ErrorIs(t, err, ErrTaskRecursion)
}

func TestGuardRunStackRejectsIndirectReentry(t *testing.T) {
	taskfile := filepath.Join(t.TempDir(), GeneratedTaskfileName)

	assignment, err := GuardRunStack(taskfile, []string{"lint"})
	require.NoError(t, err)
	setRunStack(t, assignment)

	assignment, err = GuardRunStack(taskfile, []string{"install"})
	require.NoError(t, err)
	setRunStack(t, assignment)

	_, err = GuardRunStack(taskfile, []string{"lint"})
	require.ErrorIs(t, err, ErrTaskRecursion)
}

func TestGuardRunStackAllowsRootTaskfileHandoff(t *testing.T) {
	dir := t.TempDir()

	assignment, err := GuardRunStack(filepath.Join(dir, "Taskfile.yaml"), []string{"start"})
	require.NoError(t, err)
	setRunStack(t, assignment)

	// The committed root Taskfile's `start` shells back into `dr run start`,
	// which resolves to the generated Taskfile: a different file, not a loop.
	_, err = GuardRunStack(filepath.Join(dir, GeneratedTaskfileName), []string{"start"})
	require.NoError(t, err)
}

func TestGuardRunStackTreatsDefaultTaskAsAnEntry(t *testing.T) {
	taskfile := filepath.Join(t.TempDir(), GeneratedTaskfileName)

	assignment, err := GuardRunStack(taskfile, nil)
	require.NoError(t, err)
	setRunStack(t, assignment)

	_, err = GuardRunStack(taskfile, nil)
	require.ErrorIs(t, err, ErrTaskRecursion)
}

func TestGuardRunStackIsolatesDifferentProjects(t *testing.T) {
	assignment, err := GuardRunStack(filepath.Join(t.TempDir(), GeneratedTaskfileName), []string{"lint"})
	require.NoError(t, err)
	setRunStack(t, assignment)

	_, err = GuardRunStack(filepath.Join(t.TempDir(), GeneratedTaskfileName), []string{"lint"})
	require.NoError(t, err)
}
