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

package login

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_HasTimeoutFlag(t *testing.T) {
	cmd := Cmd()

	f := cmd.Flags().Lookup("timeout")
	require.NotNil(t, f, "dr auth login must expose --timeout")
	assert.Equal(t, "0s", f.DefValue, "zero default means DefaultLoginTimeout applies")

	require.NoError(t, cmd.Flags().Set("timeout", "30s"))

	got, err := cmd.Flags().GetDuration("timeout")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, got)
}

func TestCmd_HasNoBrowserFlag(t *testing.T) {
	assert.NotNil(t, Cmd().Flags().Lookup("no-browser"), "dr auth login must keep --no-browser")
}
