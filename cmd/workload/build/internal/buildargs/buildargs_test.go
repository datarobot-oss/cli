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

package buildargs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePositional_TwoArgs(t *testing.T) {
	// With two args we never touch .wapi: the first is the artifact id,
	// the second is the build id, and we should not error even if no
	// .wapi project exists.
	t.Chdir(t.TempDir())

	artifactID, buildID, err := ResolvePositional([]string{"art-explicit", "b-1"})
	require.NoError(t, err)
	assert.Equal(t, "art-explicit", artifactID)
	assert.Equal(t, "b-1", buildID)
}

func TestResolvePositional_OneArgNoWAPIFails(t *testing.T) {
	t.Chdir(t.TempDir())

	_, _, err := ResolvePositional([]string{"b-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .wapi project")
}

func TestResolvePositional_WrongArity(t *testing.T) {
	_, _, err := ResolvePositional([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 or 2 positional arguments")

	_, _, err = ResolvePositional([]string{"a", "b", "c"})
	require.Error(t, err)
}

func TestResolveOptional_ExplicitArg(t *testing.T) {
	// With one arg we never need .wapi.
	t.Chdir(t.TempDir())

	id, err := ResolveOptional([]string{"art-explicit"})
	require.NoError(t, err)
	assert.Equal(t, "art-explicit", id)
}

func TestResolveOptional_NoArgsNoWAPIFails(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := ResolveOptional([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .wapi project")
}
