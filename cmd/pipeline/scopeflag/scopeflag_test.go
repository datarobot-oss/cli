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

package scopeflag

import (
	"io"
	"testing"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCmd builds a cobra command with the shared flags bound, so tests can
// drive Resolve() through real flag parsing.
func newCmd() (*cobra.Command, *Flags) {
	flags := &Flags{}

	cmd := &cobra.Command{
		Use: "test",
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}

	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	flags.Bind(cmd)

	return cmd, flags
}

func TestFlags_Resolve_DefaultDraft(t *testing.T) {
	cmd, flags := newCmd()

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	scope, version, err := flags.Resolve(cmd)
	require.NoError(t, err)
	assert.Equal(t, pipeline.ScopeDraft, scope)
	assert.Nil(t, version)
}

func TestFlags_Resolve_VersionImpliesLocked(t *testing.T) {
	cmd, flags := newCmd()

	cmd.SetArgs([]string{"--version=4"})
	require.NoError(t, cmd.Execute())

	scope, version, err := flags.Resolve(cmd)
	require.NoError(t, err)
	assert.Equal(t, pipeline.ScopeLocked, scope)
	require.NotNil(t, version)
	assert.Equal(t, 4, *version)
}

func TestFlags_Resolve_ExplicitDraftWithVersionErrors(t *testing.T) {
	cmd, flags := newCmd()

	cmd.SetArgs([]string{"--scope=draft", "--version=1"})
	require.NoError(t, cmd.Execute())

	_, _, err := flags.Resolve(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "draft cannot be combined")
}

func TestFlags_Resolve_LockedRequiresVersion(t *testing.T) {
	cmd, flags := newCmd()

	cmd.SetArgs([]string{"--scope=locked"})
	require.NoError(t, cmd.Execute())

	_, _, err := flags.Resolve(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --version")
}

func TestFlags_Resolve_VersionZeroIsRespected(t *testing.T) {
	// 0 is the int zero-value, but the user explicitly passed --version=0
	// so we should treat it as locked v0 (server will validate).
	cmd, flags := newCmd()

	cmd.SetArgs([]string{"--version=0"})
	require.NoError(t, cmd.Execute())

	scope, version, err := flags.Resolve(cmd)
	require.NoError(t, err)
	assert.Equal(t, pipeline.ScopeLocked, scope)
	require.NotNil(t, version)
	assert.Equal(t, 0, *version)
}

func TestFlags_Bind_RegistersAllFlags(t *testing.T) {
	cmd, _ := newCmd()

	for _, name := range []string{"pipeline", "scope", "version"} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected --%s to be registered", name)
	}
}
