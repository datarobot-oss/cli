// Copyright {{current_year}} DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type VersionFormat string

var _ pflag.Value = (*VersionFormat)(nil)

const (
	VersionFormatJSON VersionFormat = "json"
	VersionFormatText VersionFormat = "text"
)

func (v *VersionFormat) String() string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func (v *VersionFormat) Set(s string) error {
	switch s {
	case string(VersionFormatJSON), string(VersionFormatText):
		*v = VersionFormat(s)
		return nil
	}

	return fmt.Errorf("invalid format %q (must be %q or %q)",
		s, VersionFormatJSON, VersionFormatText)
}

// Type is used by the shell completion generator
func (v *VersionFormat) Type() string {
	return "VersionFormat"
}

type versionOptions struct {
	format VersionFormat
}

var options versionOptions

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the DataRobot CLI version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), getVersion(options))
		return nil
	},
}

func init() {
	options.format = VersionFormatText

	VersionCmd.Flags().VarP(
		&options.format,
		"format",
		"f",
		fmt.Sprintf("Output format (options: %s, %s)", VersionFormatJSON, VersionFormatText),
	)

	VersionCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{string(VersionFormatJSON), string(VersionFormatText)}, cobra.ShellCompDirectiveNoFileComp
	})
}

func getVersion(opts versionOptions) string {
	if opts.format == VersionFormatJSON {
		b, _ := json.Marshal(version.Info)

		return string(b)
	}

	return fmt.Sprintf("DataRobot CLI version: %s", version.FullVersion)
}
