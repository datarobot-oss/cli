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

	oidcCfg, useOIDC := resolveOIDCConfig(cmd)
	if useOIDC {
		return runOIDCLogin(cmd, oidcCfg, noBrowser)
	}

	key, err := auth.RunBrowserLoginWith(cmd.Context(), datarobotHost, auth.LoginOptions{
		NoBrowser: noBrowser,
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

// resolveOIDCConfig decides whether this login goes straight to an identity
// provider. Flags win over DATAROBOT_CLI_OAUTH_* variables and the profile's
// saved settings. A profile with a saved issuer keeps using it on a plain
// `dr auth login`; --oauth=false forces the API-key hand-off.
func resolveOIDCConfig(cmd *cobra.Command) (auth.OIDCConfig, bool) {
	cfg, saved := auth.OIDCConfigFromConfig()

	if issuer, _ := cmd.Flags().GetString(issuerFlag); issuer != "" {
		cfg.Issuer = issuer
	}

	if clientID, _ := cmd.Flags().GetString(clientIDFlag); clientID != "" {
		cfg.ClientID = clientID
	}

	if scopes, _ := cmd.Flags().GetString(scopesFlag); scopes != "" {
		cfg.Scopes = strings.Fields(scopes)
	}

	if redirectURI, _ := cmd.Flags().GetString(redirectURIFlag); redirectURI != "" {
		cfg.RedirectURI = redirectURI
	}

	if cmd.Flags().Changed(oauthFlag) {
		useOAuth, _ := cmd.Flags().GetBool(oauthFlag)

		return cfg.WithDefaults(), useOAuth
	}

	return cfg.WithDefaults(), saved || cmd.Flags().Changed(issuerFlag)
}

// runOIDCLogin signs in at the identity provider and saves the access token as
// the profile's token, plus the OIDC settings for the next login.
func runOIDCLogin(cmd *cobra.Command, cfg auth.OIDCConfig, noBrowser bool) error {
	cmd.SilenceUsage = true

	if err := cfg.Validate(); err != nil {
		log.Error(err)

		return err
	}

	log.Infof("Signing in at %s", cfg.Issuer)

	token, err := auth.RunOIDCLogin(cmd.Context(), cfg, auth.LoginOptions{NoBrowser: noBrowser})
	if err != nil {
		log.Error(err)

		return err
	}

	viperx.Set(config.DataRobotAPIKey, token)
	auth.StoreOIDCConfig(cfg)

	if err := auth.PersistOIDCConfig(); err != nil {
		log.Error(err)

		return err
	}

	if err := auth.WriteConfigFile(); err != nil {
		log.Error(err)

		return err
	}

	return nil
}

const (
	oauthFlag       = "oauth"
	issuerFlag      = "issuer"
	clientIDFlag    = "client-id"
	scopesFlag      = "scopes"
	redirectURIFlag = "redirect-uri"
)

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

With --oauth (or --issuer), the CLI signs you in at your identity provider
directly (Okta, Entra ID, Keycloak, ...) with authorization code + PKCE, and
stores the IdP's access token instead of an API key. For DataRobot deployments
whose gateway trusts that IdP. The issuer, client id, scopes and redirect URI
are saved in the profile, so later logins need no flags:

  dr auth login --oauth --issuer https://example.okta.com/oauth2/default --client-id 0oa...
  dr auth login

The redirect URI (default http://localhost:51164/) must be registered on the
IdP's app. Settings can also come from DATAROBOT_CLI_OAUTH_ISSUER,
DATAROBOT_CLI_OAUTH_CLIENT_ID, DATAROBOT_CLI_OAUTH_SCOPES and
DATAROBOT_CLI_OAUTH_REDIRECT_URI.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          RunE,
		Annotations: map[string]string{
			// login is expected to create a not-yet-existing --profile.
			config.ProfileCreateAnnotationKey: "true",
		},
	}

	// Read directly from cobra rather than binding to viper: this is a transient
	// per-invocation flag and must never be persisted to drconfig.yaml.
	cmd.Flags().Bool("no-browser", false, "print the login link instead of opening a browser")

	// OIDC flags are transient too: runOIDCLogin persists the resolved values
	// itself, under the OAuth config keys, so nothing here binds to viper.
	cmd.Flags().Bool(oauthFlag, false, "sign in at an identity provider directly (OIDC authorization code + PKCE)")
	cmd.Flags().String(issuerFlag, "", "OIDC issuer URL of the identity provider (implies --oauth)")
	cmd.Flags().String(clientIDFlag, "", "OIDC client id of the CLI's app registration at the identity provider")
	cmd.Flags().String(scopesFlag, "", `space-separated scopes to request (default "openid profile")`)
	cmd.Flags().String(redirectURIFlag, "", "loopback redirect URI registered at the identity provider (default http://localhost:51164/)")

	return cmd
}
