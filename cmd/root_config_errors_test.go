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

package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runWithConfigInitError builds a tree whose config initializer always fails
// with initErr, executes the given args against a stub command with the given
// SilenceErrors setting, and returns what reached stdout/stderr plus the
// error executeRoot returned.
func runWithConfigInitError(t *testing.T, cmdSilencesErrors bool, args []string, initErr error) (string, string, error) {
	t.Helper()
	t.Setenv(outputFormatEnvVar, "")

	root := NewIsolatedRootFactory(WithConfigInitializer(func(_ *cobra.Command) error {
		return initErr
	})).Build()

	stub := &cobra.Command{
		Use:           "stub",
		Args:          cobra.NoArgs,
		SilenceErrors: cmdSilencesErrors,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}

	root.AddCommand(stub)

	var stdout, stderr bytes.Buffer

	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := executeRoot(context.Background(), root.Command, args)

	return stdout.String(), stderr.String(), err
}

// TestConfigInitError_SilencedCommandPrintsOnce is the regression for the
// swallowed-config-error defect: `auth check`, `auth logout`, and `auth login`
// silence errors, so a failing config initialization (unknown --profile or an
// unparsable drconfig.yaml) used to exit 1 with both streams empty. The root
// must report the failure itself, exactly once, in cobra's error style.
func TestConfigInitError_SilencedCommandPrintsOnce(t *testing.T) {
	initErr := &config.UnknownProfileError{Name: "missing", Known: []string{"east", "west"}}

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "plain", args: []string{"stub"}},
		{name: "with --profile", args: []string{"--profile", "missing", "stub"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := runWithConfigInitError(t, true, tt.args, initErr)

			require.Error(t, err)
			require.ErrorIs(t, err, initErr, "the typed error must surface unwrapped")
			assert.Empty(t, stdout)
			assert.Equal(t, 1, strings.Count(stderr, `unknown profile "missing"`), "the error must print exactly once")
			assert.Contains(t, stderr, "east, west")
			assert.NotContains(t, stderr, "failed to read config file", "no wrapper noise around unknown-profile errors")
		})
	}
}

// TestConfigInitError_NonSilencedCommandPrintsOnce characterizes the existing
// behavior for commands that do not silence errors (e.g. auth profile show):
// cobra reports the PersistentPreRunE failure once, and the root must not add
// a second copy.
func TestConfigInitError_NonSilencedCommandPrintsOnce(t *testing.T) {
	initErr := &config.UnknownProfileError{Name: "missing", Known: []string{"east", "west"}}

	stdout, stderr, err := runWithConfigInitError(t, false, []string{"stub"}, initErr)

	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, 1, strings.Count(stderr, `unknown profile "missing"`), "cobra prints the error once; the root must not duplicate it")
	assert.Contains(t, stderr, "Error:")
}

// TestConfigInitError_JSONWritesEnvelopeOnce covers JSON mode for silenced
// commands: executeRoot's envelope printer is the single reporter, so stdout
// stays pure and the structured error appears once on stderr.
func TestConfigInitError_JSONWritesEnvelopeOnce(t *testing.T) {
	initErr := &config.UnknownProfileError{Name: "missing", Known: []string{"east", "west"}}

	stdout, stderr, err := runWithConfigInitError(t, true, []string{"stub", "--output-format", "json"}, initErr)

	require.Error(t, err)
	assert.Empty(t, stdout, "JSON stdout must stay pure on a config-init failure")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(stderr), "{"), "stderr must be the JSON envelope, not cobra's Error: line")
	assert.Equal(t, 1, strings.Count(stderr, `unknown profile \"missing\"`), "the structured error must appear exactly once")
	assert.NotContains(t, stderr, "Error:")
}

// TestConfigInitError_UnparsableConfigSurfaced covers the non-unknown-profile
// config failure (e.g. an unparsable drconfig.yaml) on a silenced command:
// the cause is reported once, without leaking the file's contents.
func TestConfigInitError_UnparsableConfigSurfaced(t *testing.T) {
	initErr := errors.New("While parsing config: yaml: line 2: could not find expected ':'")

	stdout, stderr, err := runWithConfigInitError(t, true, []string{"stub"}, initErr)

	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, 1, strings.Count(stderr, "While parsing config"), "the config cause must print exactly once")
}

// TestConfigInitError_PreservesCommandRenderedErrors guards the boundary of
// the new reporting: only config-initialization failures are affected.
// Commands that render their own RunE error keep their existing behavior.
func TestConfigInitError_PreservesCommandRenderedErrors(t *testing.T) {
	t.Setenv(outputFormatEnvVar, "")

	root := NewIsolatedRootFactory().Build()

	stub := &cobra.Command{
		Use:           "stub",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return cli.ErrSilent
		},
	}

	root.AddCommand(stub)

	var stdout, stderr bytes.Buffer

	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := executeRoot(context.Background(), root.Command, []string{"stub"})

	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Empty(t, stderr, "a silenced command's own rendered error gets no extra root print")
}
