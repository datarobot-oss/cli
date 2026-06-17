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
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

var _ pflag.Value = (*OutputFormat)(nil)

func (f *OutputFormat) String() string {
	if f == nil {
		return ""
	}

	return string(*f)
}

func (f *OutputFormat) Set(s string) error {
	switch s {
	case string(OutputFormatText), string(OutputFormatJSON):
		*f = OutputFormat(s)

		return nil
	}

	return fmt.Errorf("invalid output format %q: use %s or %s", s, OutputFormatText, OutputFormatJSON)
}

func (f *OutputFormat) Type() string {
	return "format"
}

func AddFlag(cmd *cobra.Command, dest *OutputFormat) {
	*dest = OutputFormatText

	cmd.Flags().Var(dest, "output-format", fmt.Sprintf("Output format (%s, %s)", OutputFormatText, OutputFormatJSON))
}

func AddPersistentFlag(cmd *cobra.Command, dest *OutputFormat) {
	*dest = OutputFormatText

	cmd.PersistentFlags().Var(dest, "output-format", fmt.Sprintf("Output format (%s, %s)", OutputFormatText, OutputFormatJSON))
}

func GetFormat(cmd *cobra.Command) OutputFormat {
	if cmd == nil {
		return OutputFormatText
	}

	local := cmd.LocalFlags().Lookup("output-format")
	if local != nil && local.Changed {
		return OutputFormat(local.Value.String())
	}

	inherited := cmd.InheritedFlags().Lookup("output-format")
	if inherited != nil && inherited.Changed {
		return OutputFormat(inherited.Value.String())
	}

	f := cmd.Flags().Lookup("output-format")
	if f != nil {
		return OutputFormat(f.Value.String())
	}

	return OutputFormatText
}
