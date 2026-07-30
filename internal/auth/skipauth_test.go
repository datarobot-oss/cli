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

package auth

import (
	"context"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSkipAuth_BoundFlagShortCircuits binds --skip-auth exactly as cmd/root.go
// does and checks that EnsureAuthenticated honours it.
//
// The previous code bound the flag as "skip-auth" but read "skip_auth". Viper
// lowercases keys yet does not treat "-" and "_" as interchangeable for lookups,
// so the flag was silently ignored: DATAROBOT_CLI_SKIP_AUTH worked (AutomaticEnv
// applies the "-" to "_" replacer when deriving env names) while the flag did
// not. A user passing --skip-auth on a fresh install was still dragged into the
// interactive login. Setting the viper key directly, as the older test did, could
// not catch that - only going through a bound pflag can.
func TestSkipAuth_BoundFlagShortCircuits(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// No credentials at all: without the short circuit this would start a login.
	viperx.Set(config.DataRobotAPIKey, "")
	t.Setenv("DATAROBOT_API_TOKEN", "")
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")

	flags := pflag.NewFlagSet("root", pflag.ContinueOnError)
	flags.Bool(config.SkipAuthKey, false, "skip authentication checks")

	require.NoError(t, flags.Parse([]string{"--" + config.SkipAuthKey}))
	require.NoError(t, viperx.BindPFlag(config.SkipAuthKey, flags.Lookup(config.SkipAuthKey)))

	assert.True(t, viperx.GetBool(config.SkipAuthKey),
		"the bound --skip-auth flag must be readable under the key it was bound to")

	assert.True(t, EnsureAuthenticated(context.Background()),
		"--skip-auth must short-circuit authentication without contacting DataRobot")
}

// TestSkipAuth_KeyMatchesFlagName guards the invariant that broke: the viper key
// and the flag name are the same string.
func TestSkipAuth_KeyMatchesFlagName(t *testing.T) {
	flags := pflag.NewFlagSet("root", pflag.ContinueOnError)
	flags.Bool("skip-auth", false, "skip authentication checks")

	assert.NotNil(t, flags.Lookup(config.SkipAuthKey),
		"config.SkipAuthKey must name the --skip-auth flag exactly; underscores do not resolve")
}
