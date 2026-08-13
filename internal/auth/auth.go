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

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/tui"
	"github.com/datarobot/cli/tui/hostpicker"
	"github.com/spf13/cobra"
)

// APIKeyCallbackFunc is a variable that holds the function for retrieving API keys.
// This can be overridden in tests to mock the browser-based authentication flow.
var APIKeyCallbackFunc = RunBrowserLogin

// AuthCallbackURL returns the DataRobot URL the user must visit to authorize the CLI.
func AuthCallbackURL(datarobotHost string) string {
	return datarobotHost + "/account/developer-tools?cliRedirect=true"
}

// ErrEnvCredentialsNotSet is returned when environment credentials are not fully configured.
var ErrEnvCredentialsNotSet = errors.New("environment credentials not set")

// PrintUnsetTokenInstructions prints platform-specific instructions for unsetting DATAROBOT_API_TOKEN.
func PrintUnsetTokenInstructions() {
	FprintUnsetTokenInstructions(os.Stdout)
}

// FprintUnsetTokenInstructions writes platform-specific instructions for
// unsetting DATAROBOT_API_TOKEN to w.
func FprintUnsetTokenInstructions(w io.Writer) {
	base, info := writerStyles(w)

	fmt.Fprint(w, info.Render("  unset DATAROBOT_API_TOKEN"))
	fmt.Fprint(w, base.Render(" (or "))
	fmt.Fprint(w, info.Render("Remove-Item Env:\\DATAROBOT_API_TOKEN"))
	fmt.Fprintln(w, base.Render(" on Windows)"))
}

// writerStyles binds the shared text styles to w's renderer. The package
// defaults probe stdout for color support, so styled text sent to a different
// stream (stderr redirected to a file while stdout is a TTY, or a buffer in
// tests) would otherwise carry ANSI codes the destination cannot display.
func writerStyles(w io.Writer) (base, info lipgloss.Style) {
	r := lipgloss.NewRenderer(w)

	return tui.BaseTextStyle.Renderer(r), tui.InfoStyle.Renderer(r)
}

// EnvCredentials holds environment variable authentication credentials.
type EnvCredentials struct {
	Endpoint string
	Token    string
	// EndpointVar is the environment variable Endpoint was read from:
	// DATAROBOT_ENDPOINT, or the SDK-style DATAROBOT_API_ENDPOINT fallback.
	// Error messages name it so the user fixes the variable that is actually
	// set.
	EndpointVar string
}

// endpointVarName returns EndpointVar, defaulting to DATAROBOT_ENDPOINT for
// zero-value credentials constructed outside GetEnvCredentials.
func (c *EnvCredentials) endpointVarName() string {
	if c.EndpointVar == "" {
		return "DATAROBOT_ENDPOINT"
	}

	return c.EndpointVar
}

// GetEnvCredentials reads DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN from environment.
// Falls back to DATAROBOT_API_ENDPOINT if DATAROBOT_ENDPOINT is not set.
//
// Values are returned verbatim; surrounding quotes are NOT stripped. The
// DataRobot Python and R SDKs read these variables the same way, so stripping
// here would let the CLI silently succeed where other SDKs fail. Callers that
// need to report why env credentials failed should use ValidateEndpoint to
// distinguish a malformed endpoint from an invalid token.
func GetEnvCredentials() EnvCredentials {
	endpointVar := "DATAROBOT_ENDPOINT"

	endpoint := os.Getenv(endpointVar)
	if endpoint == "" {
		endpointVar = "DATAROBOT_API_ENDPOINT"
		endpoint = os.Getenv(endpointVar)
	}

	return EnvCredentials{
		Endpoint:    endpoint,
		Token:       os.Getenv("DATAROBOT_API_TOKEN"),
		EndpointVar: endpointVar,
	}
}

// ValidateEndpoint returns a non-nil error when endpoint is not a usable
// DataRobot URL (for example, it is wrapped in literal surrounding quotes that
// make url.Parse fail). It lets callers distinguish a malformed
// DATAROBOT_ENDPOINT from an expired/invalid token when reporting why
// environment credentials failed verification. An empty endpoint returns nil
// (that case is ErrEnvCredentialsNotSet, handled by the caller).
func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}

	_, err := config.SchemeHostOnly(endpoint)

	return err
}

// ReportEnvCredentialsError writes a classified explanation of why an
// explicitly supplied DATAROBOT_ENDPOINT/DATAROBOT_API_TOKEN pair failed
// verification: a network timeout, a malformed endpoint, an unreachable
// endpoint, or an invalid token. Shared by EnsureAuthenticated (writing to
// stderr) and `dr auth check`.
func ReportEnvCredentialsError(w io.Writer, creds *EnvCredentials, err error) {
	base, info := writerStyles(w)

	if errors.Is(err, context.DeadlineExceeded) {
		envDatarobotHost, _ := config.SchemeHostOnly(creds.Endpoint)

		fmt.Fprint(w, base.Render("❌ Connection to "))
		fmt.Fprint(w, info.Render(envDatarobotHost))
		fmt.Fprintln(w, base.Render(" timed out. Check your network and try again."))

		return
	}

	// Distinguish a malformed endpoint from an invalid token so the user
	// fixes the right thing. A quoted endpoint (e.g. from running
	// `$(dr auth export)` instead of `eval "$(dr auth export)"`) fails here
	// and must not be reported as a token problem.
	if epErr := ValidateEndpoint(creds.Endpoint); epErr != nil {
		fmt.Fprintln(w, base.Render("❌ "+creds.endpointVarName()+" environment variable is invalid: "+epErr.Error()))
		fmt.Fprintln(w, base.Render("Set it to a valid DataRobot URL and try again."))

		return
	}

	// VerifyToken uses the endpoint value exactly as the user set it, but
	// ValidateEndpoint above is more forgiving: it accepts a bare host with
	// no scheme and trims stray whitespace. The two checks below catch the
	// values that slipped through that gap, so they are reported as a bad
	// endpoint instead of as a connection failure naming a URL the CLI never
	// actually requested.
	var urlErr *url.Error

	hasURLErr := errors.As(err, &urlErr)

	if !strings.Contains(strings.TrimSpace(creds.Endpoint), "://") {
		fmt.Fprintln(w, base.Render("❌ "+creds.endpointVarName()+" environment variable is invalid: missing URL scheme (https://...)"))
		fmt.Fprintln(w, base.Render("Set it to a valid DataRobot URL and try again."))

		return
	}

	if hasURLErr && urlErr.Op == "parse" {
		fmt.Fprintln(w, base.Render("❌ "+creds.endpointVarName()+" environment variable is invalid: "+urlErr.Err.Error()))
		fmt.Fprintln(w, base.Render("Set it to a valid DataRobot URL and try again."))

		return
	}

	// A transport-level failure (connection refused, DNS, TLS, proxy) means
	// the instance was never reached and the token was never judged, so
	// advising the user to unset the token would point them at the wrong
	// thing. VerifyToken wraps transport errors in *url.Error; a rejected
	// token comes back as a plain error from the HTTP status check.
	if hasURLErr {
		envDatarobotHost, _ := config.SchemeHostOnly(creds.Endpoint)

		fmt.Fprint(w, base.Render("❌ Could not connect to "))
		fmt.Fprint(w, info.Render(envDatarobotHost))
		fmt.Fprintln(w, base.Render(": "+urlErr.Err.Error()))
		fmt.Fprintln(w, base.Render("Check "+creds.endpointVarName()+" and your network, then try again."))

		return
	}

	fmt.Fprintln(w, base.Render("❌ DATAROBOT_API_TOKEN environment variable is invalid or expired."))
	fmt.Fprintln(w, base.Render("Unset it and try again:"))
	FprintUnsetTokenInstructions(w)
}

// reportStoredProfileNotUsed names both sides of the substitution this CLI
// refuses to make: the endpoint the environment asked for and the stored
// profile it will NOT fall back to. Printed only when a stored profile exists,
// since otherwise there is nothing to substitute.
func reportStoredProfileNotUsed(w io.Writer, creds *EnvCredentials) {
	storedHost := config.GetBaseURL()
	if storedHost == "" {
		return
	}

	requestedHost := creds.Endpoint
	if host, err := config.SchemeHostOnly(creds.Endpoint); err == nil {
		requestedHost = host
	}

	base, info := writerStyles(w)

	fmt.Fprint(w, base.Render("Environment credentials for "))
	fmt.Fprint(w, info.Render(requestedHost))
	fmt.Fprint(w, base.Render(" failed to verify; not falling back to the stored profile for "))
	fmt.Fprint(w, info.Render(storedHost))
	fmt.Fprintln(w, base.Render("."))
}

// VerifyEnvCredentials checks if environment variable credentials are valid.
// Returns credentials and nil error if valid, credentials and error otherwise.
func VerifyEnvCredentials(ctx context.Context) (*EnvCredentials, error) {
	creds := GetEnvCredentials()
	if creds.Endpoint == "" || creds.Token == "" {
		return &creds, ErrEnvCredentialsNotSet
	}

	err := config.VerifyToken(ctx, creds.Endpoint, creds.Token)

	return &creds, err
}

// EnsureAuthenticatedE checks if valid authentication exists, and if not,
// triggers the login flow automatically (see EnsureAuthenticated for the
// exceptions). Returns an error if authentication fails, suitable for use in
// Cobra PreRunE hooks.
func EnsureAuthenticatedE(cmd *cobra.Command, _ []string) error {
	if !EnsureAuthenticated(cmd.Context()) {
		return errors.New("authentication failed")
	}

	return nil
}

// EnsureAuthenticated checks if valid authentication exists, and if not,
// triggers the login flow automatically. Returns true if authentication
// is valid or was successfully obtained.
//
// A complete DATAROBOT_ENDPOINT/DATAROBOT_API_TOKEN pair that fails
// verification returns false without falling back to the stored profile and
// without starting the login flow: environment credentials are an explicit
// instance request, and substituting the profile would silently run against
// the wrong instance.
func EnsureAuthenticated(ctx context.Context) bool { //nolint: cyclop
	if viperx.GetBool(config.SkipAuthKey) {
		log.Warn("Authentication checks are disabled via the '--skip-auth' flag. This may cause API calls to fail.")

		return true
	}

	// bindValidAuthEnv binds DATAROBOT ENDPOINT/API_TOKEN to viper config only if these credentials are valid
	creds, envErr := VerifyEnvCredentials(ctx)
	if envErr == nil {
		// Now map other environment variables to config keys
		// such as those used by the DataRobot platform or other SDKs
		// and clients. If the DATAROBOT_CLI equivalents are not set,
		// then Viper will fallback to these
		_ = viperx.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
		_ = viperx.BindEnv("token", "DATAROBOT_API_TOKEN")

		return true
	}

	// A complete pair of environment credentials is an explicit request for
	// that instance (the same precedence `dr auth export` documents). Falling
	// back to the stored profile here would silently swap instances, so fail
	// loudly instead and never touch the profile.
	if !errors.Is(envErr, ErrEnvCredentialsNotSet) {
		ReportEnvCredentialsError(os.Stderr, creds, envErr)
		reportStoredProfileNotUsed(os.Stderr, creds)

		return false
	}

	datarobotHost := GetBaseURLOrAsk()
	if datarobotHost == "" {
		// Appropriate error message was already displayed in GetBaseURLOrAsk() and SetURLAction()
		return false
	}

	_, viperErr := config.GetAPIKey(ctx)
	if viperErr == nil {
		// Valid token exists in viper config file
		return true
	}

	skipAuthFlow := false

	if creds.Token != "" {
		// Partial env: DATAROBOT_API_TOKEN is set but no endpoint accompanies
		// it (a complete pair was handled above), so the token was never
		// verified. Point at it rather than starting a login flow that would
		// shadow it.
		fmt.Println(tui.BaseTextStyle.Render("Your DATAROBOT_API_TOKEN environment variable is set"))
		fmt.Println(tui.BaseTextStyle.Render("without a DATAROBOT_ENDPOINT. Set that too, or unset the token:"))
		PrintUnsetTokenInstructions()

		skipAuthFlow = true
	}

	if errors.Is(viperErr, context.DeadlineExceeded) {
		fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
		fmt.Print(tui.InfoStyle.Render(datarobotHost))
		fmt.Println(tui.BaseTextStyle.Render(" from dr cli config timed out."))
		fmt.Println(tui.BaseTextStyle.Render("Check your network and try again."))

		skipAuthFlow = true
	}

	if skipAuthFlow {
		return false
	}

	// No valid token, attempt to get one
	log.Warn("No valid API key found. Starting authentication flow...")

	// Auto-retrieve new credentials without prompting
	viperx.Set(config.DataRobotAPIKey, "")

	key, err := APIKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key.", "error", err)
		return false
	}

	viperx.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	err = WriteConfigFileSilent()
	if err != nil {
		log.Error("Failed to write config file.", "error", err)
		return false
	}

	log.Info("Authentication successful")

	return true
}

func WriteConfigFileSilent() error {
	// Only persist allowlisted keys (see config.PersistableKeys). This avoids
	// writing transient flags such as --yes back to drconfig.yaml.
	err := config.UpdateConfigFile()
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func WriteConfigFile() error {
	err := WriteConfigFileSilent()
	if err != nil {
		return err
	}

	fmt.Println("Config file written successfully.")

	return nil
}

// printSetURLPrompt draws the non-interactive environment menu.
//
// The region icons are the single-codepoint globes from tui/hostpicker rather
// than country flags: a flag is a regional-indicator pair that Windows paints as
// two letter boxes (4 columns) while every width calculation says 2, which both
// hides the icon and knocks this hand-aligned box out of true. See the comment on
// hostpicker's icon constants.
func printSetURLPrompt() {
	fmt.Println("🌐 DataRobot URL Configuration")
	fmt.Println("")
	fmt.Println("Choose your DataRobot environment:")
	fmt.Println("")
	fmt.Println("┌────────────────────────────────────────────────────────┐")
	fmt.Printf("│  [1] %s US Cloud        https://app.datarobot.com      │\n", hostpicker.USRegionIcon)
	fmt.Printf("│  [2] %s EU Cloud        https://app.eu.datarobot.com   │\n", hostpicker.EURegionIcon)
	fmt.Printf("│  [3] %s Japan Cloud     https://app.jp.datarobot.com   │\n", hostpicker.JPRegionIcon)
	fmt.Printf("│      %s Custom          Enter your custom URL          │\n", hostpicker.CustomRegionIcon)
	fmt.Println("└────────────────────────────────────────────────────────┘")
	fmt.Println("")
	fmt.Println("🔗 Don't know which one? Check your DataRobot login page URL in your browser.")
	fmt.Println("")
	fmt.Print("Enter your choice: ")
}

func askForNewHost() bool {
	datarobotHost := config.GetBaseURL()

	if len(datarobotHost) == 0 {
		return true
	}

	fmt.Printf("A DataRobot URL of %s is already present; do you want to overwrite it? (y/N): ", datarobotHost)

	selectedOption, err := reader.ReadString()
	if err != nil {
		return false
	}

	return strings.ToLower(strings.TrimSpace(selectedOption)) == "y"
}

// SetURLAction asks the user which DataRobot environment to use and stores the
// answer. It reports whether the configured URL changed.
//
// On an interactive terminal this is a TUI list (arrow keys plus Enter, US Cloud
// preselected). On a terminal with DATAROBOT_CLI_NON_INTERACTIVE set it falls back
// to the plain-text numbered prompt, which is what the expect based smoke tests
// drive - they spawn a PTY and set that variable. With no terminal at all there is
// nothing to prompt, so it prints the non-interactive instructions and gives up.
func SetURLAction() bool {
	// Every path below reads stdin, askForNewHost's overwrite prompt included, so
	// this has to come first. Without a terminal no answer is coming: on Windows
	// cancelreader discards a redirected stdin in favour of the real console and
	// then blocks forever waiting for keystrokes. See internal/misc/reader.
	if !reader.IsStdinTerminal() {
		printURLUnchanged()

		return false
	}

	if !askForNewHost() {
		printURLUnchanged()

		return false
	}

	if interactiveURLPickerAvailable() {
		return selectURLInteractively()
	}

	return readURLFromStdin()
}

// interactiveURLPickerAvailable reports whether the TUI picker can be shown.
//
// The testing.Testing() guard matches internal/misc/open: `go test` normally hands
// the binary /dev/null for stdin, but under a PTY it would not, and a test that
// launched the real picker would block until the suite timed out.
func interactiveURLPickerAvailable() bool {
	if testing.Testing() {
		return false
	}

	return reader.IsStdinTerminal() && !reader.IsNonInteractive()
}

// selectURLInteractively runs the TUI environment picker.
func selectURLInteractively() bool {
	for {
		url, ok, err := hostpicker.Select()
		if err != nil {
			log.Error("Failed to show the environment picker.", "error", err)
			printURLUnchanged()

			return false
		}

		if !ok {
			// The user pressed Esc or Ctrl-C.
			printURLUnchanged()

			return false
		}

		err = config.SetURLToConfig(url)
		if err == nil {
			return true
		}

		if errors.Is(err, config.ErrInvalidURL) {
			fmt.Println(tui.ErrorStyle.Render("Invalid URL provided. Verify your URL and try again."))

			continue
		}

		log.Error(err)
		printURLUnchanged()

		return false
	}
}

// readURLFromStdin is the non-interactive fallback: a numbered prompt read from
// stdin, so DATAROBOT_CLI_NON_INTERACTIVE and the expect-based smoke tests keep
// working.
func readURLFromStdin() bool {
	for {
		printSetURLPrompt()

		// reader.ReadString strips the newline, so an empty answer is "" - testing
		// for "\n" here would send it to SetURLToConfig and loop on "Invalid URL".
		url, err := reader.ReadString()
		if err != nil || url == "" {
			break
		}

		err = config.SetURLToConfig(url)
		if err != nil {
			if errors.Is(err, config.ErrInvalidURL) {
				fmt.Print("\nInvalid URL provided. Verify your URL and try again.\n\n")
				continue
			}

			log.Error(err)

			break
		}

		fmt.Println("Thank you for providing the URL. Validating it and retrieving your API key...")

		return true
	}

	printURLUnchanged()

	return false
}

// printURLUnchanged explains that the configured URL was left as it is and how to
// proceed non-interactively.
//
// The bare "Exiting without changing the DataRobot URL." this replaces left users
// with no next step, which is especially unhelpful when the cause was redirected
// stdin rather than a deliberate cancel. The first line depends on whether a URL is
// already configured: callers get here either because nothing was ever set or
// because the user kept what was there (declining the overwrite prompt, cancelling
// the picker), and "nothing configured" would be wrong in the latter case.
func printURLUnchanged() {
	if current := config.GetBaseURL(); len(current) > 0 {
		fmt.Print(tui.BaseTextStyle.Render("Keeping the current DataRobot URL "))
		fmt.Println(tui.InfoStyle.Render(current))
		fmt.Print(tui.BaseTextStyle.Render("Change it non-interactively with "))
	} else {
		fmt.Println(tui.BaseTextStyle.Render("No DataRobot URL was configured."))
		fmt.Print(tui.BaseTextStyle.Render("Set one non-interactively with "))
	}

	fmt.Println(tui.InfoStyle.Render("dr auth set-url <url>"))
	fmt.Print(tui.BaseTextStyle.Render("or export "))
	fmt.Print(tui.InfoStyle.Render("DATAROBOT_ENDPOINT"))
	fmt.Print(tui.BaseTextStyle.Render(" and "))
	fmt.Print(tui.InfoStyle.Render("DATAROBOT_API_TOKEN"))
	fmt.Println(tui.BaseTextStyle.Render(" to skip the login entirely."))
}

func GetBaseURLOrAsk() string {
	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		log.Warn("No DataRobot URL configured. Running auth setup...")

		SetURLAction()

		datarobotHost = config.GetBaseURL()
		if datarobotHost == "" {
			log.Error("Failed to configure the DataRobot URL.")
			return ""
		}
	}

	return datarobotHost
}
