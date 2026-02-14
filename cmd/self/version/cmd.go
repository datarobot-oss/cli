// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package version

import (
	"encoding/json"
	"fmt"

	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Format string

var _ pflag.Value = (*Format)(nil)

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

func (vf *Format) String() string {
	if vf == nil {
		return ""
	}

	return string(*vf)
}

func (vf *Format) Set(s string) error {
	switch s {
	case string(FormatJSON), string(FormatText):
		*vf = Format(s)
		return nil
	}

	return fmt.Errorf("Invalid format %q (must be %q or %q).",
		s, FormatJSON, FormatText)
}

// Type is used by the shell completion generator
func (vf *Format) Type() string {
	return "version.Format"
}

type versionOptions struct {
	format Format
	short  bool
}

func Cmd() *cobra.Command {
	var options versionOptions

	options.format = FormatText

	cmd := &cobra.Command{
		Use:   "version",
		Short: "📋 Show " + internalVersion.AppName + " version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info, err := getVersion(options)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), info)

			return nil
		},
	}

	cmd.Flags().VarP(
		&options.format,
		"format",
		"f",
		fmt.Sprintf("Output format (options: %s, %s)", FormatJSON, FormatText),
	)

	cmd.Flags().BoolVarP(&options.short, "short", "s", false, "Short format")

	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{string(FormatJSON), string(FormatText)}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func getVersion(opts versionOptions) (string, error) {
	if opts.short {
		return internalVersion.Version, nil
	}

	if opts.format == FormatJSON {
		b, err := json.Marshal(internalVersion.Info)
		if err != nil {
			return "", fmt.Errorf("Failed to marshal version info to JSON: %w", err)
		}

		return string(b), nil
	}

	return internalVersion.GetAppNameVersionText(), nil
}
