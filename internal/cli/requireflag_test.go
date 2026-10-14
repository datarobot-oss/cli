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

package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireFlagWhenChanged(t *testing.T) {
	for name, c := range map[string]struct {
		args    []string
		wantErr bool
	}{
		"neither":           {nil, false},
		"only the required": {[]string{"--use-case-id", "x"}, false},
		"both":              {[]string{"--enclave", "e", "--use-case-id", "x"}, false},
		"only the flag":     {[]string{"--enclave", "e"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "t"}
			cmd.Flags().String("enclave", "", "")
			cmd.Flags().String("use-case-id", "", "")
			require.NoError(t, cmd.Flags().Parse(c.args))

			err := RequireFlagWhenChanged(cmd, "enclave", "use-case-id")
			if c.wantErr {
				require.Error(t, err)
				assert.Equal(t, "--enclave requires --use-case-id", err.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}
