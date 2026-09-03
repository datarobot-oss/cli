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
	"context"
	"errors"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/spf13/cobra"
)

func RunE(cmd *cobra.Command, args []string) error { //nolint: cyclop
	// short-circuit if skip-auth is enabled. This allows users to avoid login prompts
	// when authentication is intentionally disabled, say if the user is offline, or in
	// a CI/CD environment, or in a script.
	if viperx.GetBool(config.SkipAuthKey) {
		err := errors.New("Login has been disabled via the '--skip-auth' flag.")
		log.Error(err)

		return err
	}

	var url string
	if len(args) > 0 {
		url = args[0]
	}

	if url != "" {
		err := config.SetURLToConfig(url)
		if err != nil {
			log.Error(err.Error())
		}
	}

	datarobotHost := auth.GetBaseURLOrAsk()
	if datarobotHost == "" {
		log.Info("💡 To set your DataRobot URL, run 'dr auth set-url'.")

		return cli.ErrSilent
	}

	token, err := config.GetAPIKey(context.Background())
	if errors.Is(err, context.DeadlineExceeded) {
		log.Errorf("Connection to %s timed out. Check your network and try again.", datarobotHost)

		return cli.ErrSilent
	}

	// If they explicitly ran 'dr auth login', just authenticate them
	if token != "" {
		log.Info("Re-authenticating with DataRobot...")
	} else {
		log.Warn("No valid API key found. Retrieving a new one...")
	}

	log.Info("💡 To change your DataRobot URL, run 'dr auth set-url'.")

	// Clear existing token and get new one
	viperx.Set(config.DataRobotAPIKey, "")

	noBrowser, _ := cmd.Flags().GetBool("no-browser")

	// Only pass an override when the user actually said something. Left nil,
	// DATAROBOT_OAUTH_ENABLED decides, and its default is off.
	var oauthOverride *bool

	if cmd.Flags().Changed("oauth") {
		oauth, _ := cmd.Flags().GetBool("oauth")
		oauthOverride = &oauth
	}

	key, err := auth.RunBrowserLoginWith(cmd.Context(), datarobotHost, auth.LoginOptions{
		NoBrowser: noBrowser,
		OAuth:     oauthOverride,
	})
	if err != nil {
		log.Error(err)

		cmd.SilenceUsage = true

		return err
	}

	if key == "" {
		return nil
	}

	viperx.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	err = auth.WriteConfigFile()
	if err != nil {
		log.Error(err)

		cmd.SilenceUsage = true

		return err
	}

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [url]",
		Short: "🔐 Log in to DataRobot in your browser.",
		Long: `Log in to DataRobot by authorizing the CLI in your browser.

This command will:
  1. Open your default browser.
  2. Redirect you to the DataRobot login page.
  3. Securely store your API key for future CLI operations.

If the browser cannot be opened, the CLI prints a link to open yourself. Pass
--no-browser to skip the browser launch entirely, which is useful over SSH.

Deployments that front their own OAuth2 authorization server — such as a
self-hosted inference stack — can be logged into with --oauth, which runs a
standard authorization-code flow with PKCE instead of the DataRobot hand-off.
The access token then never travels in a URL. Set DATAROBOT_OAUTH_ENABLED=true
to make that the default for a shell; --no-oauth forces the hand-off back on.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          RunE,
	}

	// Read directly from cobra rather than binding to viper: this is a transient
	// per-invocation flag and must never be persisted to drconfig.yaml.
	cmd.Flags().Bool("no-browser", false, "print the login link instead of opening a browser")

	// Same reasoning as --no-browser: transient, never persisted. Registered as
	// one boolean so cobra gives us --oauth and --no-oauth for free, and read
	// via Flags().Changed so "unset" stays distinguishable from "--no-oauth".
	cmd.Flags().Bool("oauth", false, "log in with OAuth2 authorization-code + PKCE (default: DATAROBOT_OAUTH_ENABLED)")

	return cmd
}
