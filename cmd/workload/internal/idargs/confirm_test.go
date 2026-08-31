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

package idargs

import (
	"bytes"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asking builds a command shaped like the mutating leaves: --yes, and a
// --output-format of its own.
func asking(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var format outputformat.OutputFormat

	cmd := &cobra.Command{Use: "stop"}
	AddYesFlag(cmd, "skip")
	outputformat.AddFlag(cmd, &format)

	var out, errOut bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	return cmd, &out, &errOut
}

func TestConfirm(t *testing.T) {
	t.Run("--yes answers it", func(t *testing.T) {
		t.Setenv(reader.NonInteractiveEnv, "")

		cmd, _, _ := asking(t)
		require.NoError(t, cmd.Flags().Set("yes", "true"))

		confirmed, err := Confirm(cmd, "go? [y/N] ", EnvMayConsent)
		require.NoError(t, err)
		assert.True(t, confirmed)
	})

	t.Run("the env var answers it, unless the caller says otherwise", func(t *testing.T) {
		t.Setenv(reader.NonInteractiveEnv, "1")
		t.Cleanup(viperx.Reset)

		cmd, _, _ := asking(t)

		confirmed, err := Confirm(cmd, "go? [y/N] ", EnvMayConsent)
		require.NoError(t, err)
		assert.True(t, confirmed)

		// The rule that keeps a CI-wide env var from consenting to an
		// irreversible delete of a workload nobody named.
		_, err = Confirm(cmd, "go? [y/N] ", EnvMayNotConsent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pass --yes to run")
		assert.NotContains(t, err.Error(), reader.NonInteractiveEnv)
	})

	// Declaring the run non-interactive has to mean "do not ask" even where it
	// does not mean "yes", or the one case that refuses it blocks on a prompt
	// nobody is there to answer.
	t.Run("a declared non-interactive run is refused, never asked", func(t *testing.T) {
		t.Setenv(reader.NonInteractiveEnv, "1")
		t.Cleanup(viperx.Reset)

		cmd, out, errOut := asking(t)

		_, err := Confirm(cmd, "go? [y/N] ", EnvMayNotConsent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "confirmation required")
		assert.Empty(t, out.String()+errOut.String(), "no question may be put")
	})

	// viper's cast takes only what strconv.ParseBool knows; the variable's own
	// reader accepts more, and the docs promise the variable, not the cast.
	t.Run("every truthy spelling of the variable counts", func(t *testing.T) {
		t.Setenv(reader.NonInteractiveEnv, "yes")
		t.Cleanup(viperx.Reset)

		cmd, _, _ := asking(t)

		confirmed, err := Confirm(cmd, "go? [y/N] ", EnvMayConsent)
		require.NoError(t, err)
		assert.True(t, confirmed)
	})

	// The blocking bug: a question printed into a JSON run breaks whatever is
	// parsing it, and stdin being a terminal says nothing about that.
	t.Run("a JSON run is non-interactive", func(t *testing.T) {
		t.Setenv(reader.NonInteractiveEnv, "")

		cmd, out, errOut := asking(t)
		require.NoError(t, cmd.Flags().Set("output-format", "json"))

		_, err := Confirm(cmd, "go? [y/N] ", EnvMayConsent)
		require.Error(t, err)

		// Named rather than the generic refusal, so this proves the JSON check
		// fired: under go test stdin is never a terminal, and the no-terminal
		// branch would answer the same way whether or not the check existed.
		assert.Contains(t, err.Error(), "--output-format json cannot ask")
		assert.Empty(t, out.String(), "no question may reach stdout")
		assert.Empty(t, errOut.String())
	})
}

// announce keys off the command's own flag, not the inherited one: delete and
// endpoint never render JSON, so a root --output-format must not cost them
// their attribution.
func TestAnnounce_IgnoresAnInheritedOutputFormat(t *testing.T) {
	t.Cleanup(viperx.Reset)
	viperx.Set("output-format", "json")

	var errOut bytes.Buffer

	cmd := &cobra.Command{Use: "delete"}
	cmd.SetErr(&errOut)

	announce(cmd, Ref{ID: boundID, Source: WorkloadIDSourceManifest, Path: "/p/.datarobot.yaml"})
	assert.Contains(t, errOut.String(), boundID)

	var format outputformat.OutputFormat

	speaking := &cobra.Command{Use: "status"}
	outputformat.AddFlag(speaking, &format)

	var quiet bytes.Buffer

	speaking.SetErr(&quiet)
	announce(speaking, Ref{ID: boundID, Source: WorkloadIDSourceManifest, Path: "/p/.datarobot.yaml"})
	assert.Empty(t, quiet.String(), "a command that does render JSON stays quiet")
}
