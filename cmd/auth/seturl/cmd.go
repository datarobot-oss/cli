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

package seturl

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// RunE configures the DataRobot endpoint, persists it, and then continues
// into authentication. A successful persistence announces where the
// credentials landed; a failed one is reported instead of being ignored, so
// the command never claims a write that did not happen.
func RunE(cmd *cobra.Command, args []string) error {
	var url string
	if len(args) > 0 {
		url = args[0]
	}

	// A bad explicit arg is reported, not silently replaced by the picker.
	if url != "" {
		if err := config.SetURLToConfig(url); err != nil {
			log.Error(err.Error())

			return cli.ErrSilent
		}

		if err := persistAndAnnounce(cmd); err != nil {
			return err
		}

		_ = auth.EnsureAuthenticatedE(cmd, args)

		return nil
	}

	if auth.SetURLAction() {
		if err := persistAndAnnounce(cmd); err != nil {
			return err
		}

		_ = auth.EnsureAuthenticatedE(cmd, args)
	}

	return nil
}

// persistAndAnnounce writes the allowlisted config keys and, only after the
// write succeeded, emits the single save notice naming the saved profile and
// the actual config file. The notice goes to the error stream, so
// --output-format json keeps stdout a parseable channel.
//
// A write failure is reported once -- by WriteConfigFileSilent's own error
// logging -- and returned as the silent sentinel, matching how this command
// reports its other RunE errors; the subsequent authentication step is
// skipped so a failed write is never followed by a success claim.
func persistAndAnnounce(cmd *cobra.Command) error {
	if err := auth.WriteConfigFileSilent(); err != nil {
		// Already reported by WriteConfigFileSilent; log again and this
		// failure would appear twice.
		return cli.ErrSilent
	}

	auth.AnnounceCredentialsSavedForCommand(cmd)

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "set-url [url]",
		SilenceErrors: true,
		SilenceUsage:  true,
		Short:         "🌐 Configure your DataRobot environment URL.",
		Long: `Configure your DataRobot environment URL with an interactive selection.

This command helps you choose the correct DataRobot environment:
  • US Cloud (most common): https://app.datarobot.com
  • EU Cloud: https://app.eu.datarobot.com
  • Japan Cloud: https://app.jp.datarobot.com
  • Custom/On-Premise: Your organization's DataRobot URL

💡 If you're unsure, check the URL you use to log in to DataRobot in your browser.`,
		Annotations: map[string]string{
			// set-url is expected to create a not-yet-existing --profile.
			config.ProfileCreateAnnotationKey: "true",
		},
		RunE: RunE,
	}

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"url": telemetry.FirstArg(args),
		}
	})

	return cmd
}
