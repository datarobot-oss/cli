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
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestFromArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat OutputFormat
		wantFound  bool
	}{
		{"no args", nil, "", false},
		{"unrelated args", []string{"run", "--verbose"}, "", false},
		{"separate value", []string{"--output-format", "json"}, OutputFormatJSON, true},
		{"equals value", []string{"--output-format=json"}, OutputFormatJSON, true},
		{"text value", []string{"--output-format", "text"}, OutputFormatText, true},
		{"after other args", []string{"run", "app", "--output-format", "json"}, OutputFormatJSON, true},
		{"last occurrence wins", []string{"--output-format", "json", "--output-format=text"}, OutputFormatText, true},
		{"value is returned unvalidated", []string{"--output-format", "yaml"}, "yaml", true},
		{"flag without a value", []string{"--output-format"}, "", false},
		{"stops at the terminator", []string{"--", "--output-format", "json"}, "", false},
		{"flag before the terminator still counts", []string{"--output-format=json", "--", "--output-format=text"}, OutputFormatJSON, true},
		{"value that merely contains json", []string{"--file", "out.json"}, "", false},
		{"deprecated -o alias is not recognised", []string{"-o", "json"}, "", false},
		{"deprecated --format alias is not recognised", []string{"--format", "json"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, found := FromArgs(tt.args)

			assert.Equal(t, tt.wantFormat, format)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func TestGetFormatFromArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		viperFormat string
		want        OutputFormat
	}{
		{"nothing set defaults to text", nil, "", OutputFormatText},
		{"args win", []string{"--output-format", "json"}, "", OutputFormatJSON},
		{"args beat the config value", []string{"--output-format", "text"}, "json", OutputFormatText},
		{"config value is the fallback", nil, "json", OutputFormatJSON},
		{"unrelated args fall back to the config value", []string{"run", "--file", "out.json"}, "json", OutputFormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viperx.Reset()
			t.Cleanup(viperx.Reset)

			if tt.viperFormat != "" {
				viperx.Set("output-format", tt.viperFormat)
			}

			cmd := &cobra.Command{Use: "plugin", DisableFlagParsing: true}

			assert.Equal(t, tt.want, GetFormatFromArgs(cmd, tt.args))
		})
	}
}
