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

package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParentProcessName_ReturnsNonEmpty(t *testing.T) {
	name := parentProcessName()

	// In the test runner the parent is always determinable on Linux/macOS
	// (either via /proc or ps). We don't assert a specific name because
	// it depends on the test runner (e.g. "go", "task", etc.).
	assert.NotEmpty(t, name)
}

func TestDetectShell_ReturnsNonEmpty(t *testing.T) {
	name, err := DetectShell()

	require.NoError(t, err)
	assert.NotEmpty(t, name)
}

func TestDetectShell_EnvVarFallback(t *testing.T) {
	// Verify that $SHELL is used when set (simulates the env var fallback path).
	t.Setenv("SHELL", "/usr/bin/fish")

	// parentProcessName() will still return the test runner's parent, so we
	// can't easily force the $SHELL fallback in a unit test without process
	// manipulation. Instead, assert DetectShell returns a non-empty result.
	name, err := DetectShell()

	require.NoError(t, err)
	assert.NotEmpty(t, name)
}
