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

package version

import (
	"encoding/json"
	"fmt"

	"github.com/datarobot/cli/internal/outputformat"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

type versionOptions struct {
	outputFormat outputformat.OutputFormat
	legacyFormat outputformat.OutputFormat
	short        bool
}

func Cmd() *cobra.Command {
	var options versionOptions

	options.outputFormat = outputformat.OutputFormatJSON

	cmd := &cobra.Command{
		Use:   "version",
		Short: "📋 Show " + internalVersion.AppName + " version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format := outputformat.GetFormat(cmd)

			// Backward compat: --format takes precedence if explicitly set
			if legacyFlag := cmd.Flags().Lookup("format"); legacyFlag != nil && legacyFlag.Changed {
				format = options.legacyFormat
			}

			info, err := getVersion(format, options.short)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), info)

			return nil
		},
	}

	cmd.Flags().VarP(
		&options.outputFormat,
		"output-format",
		"o",
		fmt.Sprintf("Output format (%s, %s)", outputformat.OutputFormatJSON, outputformat.OutputFormatText),
	)

	// Deprecated: use --output-format instead. Kept for backward compat with smoke test scripts.
	cmd.Flags().VarP(
		&options.legacyFormat,
		"format",
		"f",
		fmt.Sprintf("Output format (deprecated, use --output-format) (%s, %s)", outputformat.OutputFormatJSON, outputformat.OutputFormatText),
	)

	_ = cmd.Flags().MarkHidden("format")
	_ = cmd.Flags().MarkDeprecated("format", "use --output-format instead")

	cmd.Flags().BoolVarP(&options.short, "short", "s", false, "Print just the version number (text format only)")

	_ = cmd.RegisterFlagCompletionFunc("output-format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{string(outputformat.OutputFormatJSON), string(outputformat.OutputFormatText)}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func getVersion(format outputformat.OutputFormat, short bool) (string, error) {
	if short && format == outputformat.OutputFormatText {
		return internalVersion.Version, nil
	}

	if format == outputformat.OutputFormatJSON {
		b, err := json.Marshal(internalVersion.Info)
		if err != nil {
			return "", fmt.Errorf("Failed to marshal version info to JSON: %w", err)
		}

		return string(b), nil
	}

	return internalVersion.GetAppNameVersionText(), nil
}
