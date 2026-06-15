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

package pollflags

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterWithDefaults_AppliesDefaults(t *testing.T) {
	cmd := &cobra.Command{}

	var s Set

	RegisterWithDefaults(cmd, &s, 3*time.Second, 2*time.Minute, "wait usage")

	require.NoError(t, cmd.ParseFlags(nil))
	assert.Equal(t, 3*time.Second, s.Interval)
	assert.Equal(t, 2*time.Minute, s.Timeout)
	assert.False(t, s.Wait)
}

func TestRegisterWithDefaults_RejectsNonPositiveDurations(t *testing.T) {
	for _, args := range [][]string{
		{"--poll-interval", "0s"},
		{"--poll-interval", "-1s"},
		{"--poll-timeout", "0s"},
	} {
		cmd := &cobra.Command{}

		var s Set

		RegisterWithDefaults(cmd, &s, time.Second, time.Minute, "wait usage")

		// A non-positive cadence would turn time.Sleep into a no-op and the
		// poll into a hot loop, so it is rejected at parse time.
		err := cmd.ParseFlags(args)
		require.Errorf(t, err, "args %v", args)
		assert.Contains(t, err.Error(), "must be a positive duration")
	}
}

func TestRegisterWithDefaults_ParsesPositiveDurations(t *testing.T) {
	cmd := &cobra.Command{}

	var s Set

	RegisterWithDefaults(cmd, &s, time.Second, time.Minute, "wait usage")

	require.NoError(t, cmd.ParseFlags([]string{"--wait", "--poll-interval", "100ms", "--poll-timeout", "30s"}))
	assert.True(t, s.Wait)
	assert.Equal(t, 100*time.Millisecond, s.Interval)
	assert.Equal(t, 30*time.Second, s.Timeout)
}
