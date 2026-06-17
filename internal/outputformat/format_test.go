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

package outputformat

import (
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFormat_Set(t *testing.T) {
	cases := []struct {
		in      string
		want    OutputFormat
		wantErr bool
	}{
		{"text", OutputFormatText, false},
		{"json", OutputFormatJSON, false},
		{"yaml", "", true},
		{"", "", true},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var f OutputFormat

			err := f.Set(c.in)
			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid output format")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.want, f)
		})
	}
}

func TestGetFormat_DefaultText(t *testing.T) {
	cmd := &cobra.Command{Use: "child"}

	assert.Equal(t, OutputFormatText, GetFormat(cmd))
}

func TestGetFormat_InheritedPersistentFlag(t *testing.T) {
	var rootFormat OutputFormat

	root := &cobra.Command{Use: "root"}
	AddPersistentFlag(root, &rootFormat)

	child := &cobra.Command{
		Use:  "child",
		RunE: func(cmd *cobra.Command, _ []string) error { return nil },
	}
	root.AddCommand(child)
	root.SetArgs([]string{"child", "--output-format", "json"})

	require.NoError(t, root.Execute())
	assert.Equal(t, OutputFormatJSON, GetFormat(child))
}

func TestGetFormat_EnvironmentVariableViaViperBinding(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_OUTPUT_FORMAT", "json")

	viperx.Reset()
	viperx.SetEnvPrefix("DATAROBOT_CLI")
	viperx.AutomaticEnv()
	viperx.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	var format OutputFormat

	cmd := &cobra.Command{Use: "test"}
	AddPersistentFlag(cmd, &format)

	_ = viperx.BindPFlag("output-format", cmd.PersistentFlags().Lookup("output-format"))

	flag := cmd.PersistentFlags().Lookup("output-format")
	require.NotNil(t, flag)

	flagValue := viperx.Get("output-format")
	assert.Equal(t, "json", flagValue)

	viperx.Reset()
}
