// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package version

import (
	"encoding/json"
	"fmt"

	"github.com/datarobot/cli/internal/version"
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

	return fmt.Errorf("invalid format %q (must be %q or %q)",
		s, FormatJSON, FormatText)
}

// Type is used by the shell completion generator
func (vf *Format) Type() string {
	return "version.Format"
}

type versionOptions struct {
	format Format
}

func Cmd() *cobra.Command {
	var options versionOptions

	options.format = FormatText

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the " + version.AppName + " version information",
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

	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{string(FormatJSON), string(FormatText)}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func getVersion(opts versionOptions) (string, error) {
	if opts.format == FormatJSON {
		b, err := json.Marshal(version.Info)
		if err != nil {
			return "", fmt.Errorf("failed to marshal version info to JSON: %w", err)
		}

		return string(b), nil
	}

	return version.AppName + " version: " + version.FullVersion, nil
}
