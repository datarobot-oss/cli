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
	"net/http"
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

// ErrMissingScheme is an endpoint with no scheme. SchemeHostOnly defaults https,
// but VerifyToken dials the raw value and gets `unsupported protocol scheme ""`.
var ErrMissingScheme = errors.New("missing URL scheme (https://...)")

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

// ValidateEndpoint rejects an endpoint the CLI cannot use: one that will not
// parse (surrounding quotes), one with no scheme, or a scheme beyond http/https.
func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}

	baseURL, err := config.SchemeHostOnly(endpoint)
	if err != nil {
		return err
	}

	// Both checked here, not in SchemeHostOnly, which set-url and export share
	// and which defaults a bare host to https. VerifyToken dials the raw value.
	if !strings.Contains(strings.TrimSpace(endpoint), "://") {
		return ErrMissingScheme
	}

	if scheme, _, _ := strings.Cut(baseURL, "://"); scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q, use https://", scheme)
	}

	return nil
}

// ReportEnvCredentialsError explains why an explicit endpoint/token pair failed
// verification. Shared by EnsureAuthenticated (stderr) and `dr auth check`.
func ReportEnvCredentialsError(w io.Writer, creds *EnvCredentials, err error) {
	base, info := writerStyles(w)

	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprint(w, base.Render("❌ Connection to "))
		fmt.Fprint(w, info.Render(hostOrEndpoint(creds.Endpoint)))
		fmt.Fprintln(w, base.Render(" timed out. Check your network and try again."))

		return
	}

	// A quoted endpoint, from `$(dr auth export)` without the eval, lands here
	// and must not be reported as a token problem.
	if reason := unusableEndpoint(creds.Endpoint, err); reason != "" {
		fmt.Fprintln(w, base.Render("❌ "+creds.endpointVarName()+" environment variable is invalid: "+reason))
		fmt.Fprintln(w, base.Render("Set it to a valid DataRobot URL and try again."))

		return
	}

	if fprintServerStatus(w, creds.Endpoint, creds.endpointVarName(), err) {
		return
	}

	if fprintTransportError(w, creds.Endpoint, creds.endpointVarName(), err) {
		return
	}

	fmt.Fprintln(w, base.Render("❌ DATAROBOT_API_TOKEN environment variable is invalid or expired."))
	fmt.Fprintln(w, base.Render("Unset it and try again:"))
	FprintUnsetTokenInstructions(w)
}

// StoredEndpointName names the endpoint in messages about the stored profile.
// Not dr auth set-url: DATAROBOT_CLI_ENDPOINT can supply it and outranks the file.
const StoredEndpointName = "the configured DataRobot endpoint"

// ReportUnjudged explains a verification failure that produced no verdict on the
// credentials and reports whether it did. False means the instance rejected them.
func ReportUnjudged(w io.Writer, endpoint, endpointName string, err error) bool {
	base, info := writerStyles(w)

	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprint(w, base.Render("❌ Connection to "))
		fmt.Fprint(w, info.Render(hostOrEndpoint(endpoint)))
		fmt.Fprintln(w, base.Render(" timed out. Check your network and try again."))

		return true
	}

	// Naming the host SchemeHostOnly invented would name a URL never requested.
	if reason := unusableEndpoint(endpoint, err); reason != "" {
		fmt.Fprintln(w, base.Render("❌ "+endpointName+" is invalid: "+reason))

		return true
	}

	return fprintServerStatus(w, endpoint, endpointName, err) ||
		fprintTransportError(w, endpoint, endpointName, err)
}

// unusableEndpoint returns why the CLI cannot dial endpoint, or "" when it can.
// ValidateEndpoint trims, so VerifyToken's raw parse failure is checked too.
func unusableEndpoint(endpoint string, err error) string {
	if epErr := ValidateEndpoint(endpoint); epErr != nil {
		return reasonWithoutURL(epErr)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Op == "parse" {
		return reasonWithoutURL(urlErr)
	}

	return ""
}

// reasonWithoutURL drops the copy of the endpoint url.Parse embeds in its error,
// which would print the userinfo password every other message redacts.
func reasonWithoutURL(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}

	return err.Error()
}

// fprintTransportError explains a failure that never reached the instance and
// reports whether it did. Test DeadlineExceeded first: a timeout is a *url.Error too.
func fprintTransportError(w io.Writer, endpoint, endpointName string, err error) bool {
	var urlErr *url.Error

	if !errors.As(err, &urlErr) {
		return false
	}

	base, info := writerStyles(w)

	fmt.Fprint(w, base.Render("❌ Could not connect to "))
	fmt.Fprint(w, info.Render(hostOrEndpoint(endpoint)))
	fmt.Fprintln(w, base.Render(": "+urlErr.Err.Error()))
	fmt.Fprintln(w, base.Render("Check "+endpointName+" and your network, then try again."))

	return true
}

// fprintServerStatus explains a non-200 verification failure and reports whether
// it did. 401/403 and status-less errors return false.
func fprintServerStatus(w io.Writer, endpoint, endpointName string, err error) bool {
	var statusErr *config.HTTPStatusError

	if !errors.As(err, &statusErr) ||
		statusErr.StatusCode == http.StatusUnauthorized ||
		statusErr.StatusCode == http.StatusForbidden {
		return false
	}

	base, info := writerStyles(w)

	fmt.Fprint(w, base.Render("❌ "))
	fmt.Fprint(w, info.Render(hostOrEndpoint(endpoint)))
	fmt.Fprintln(w, base.Render(fmt.Sprintf(" answered HTTP %d, so the CLI could not verify your credentials.",
		statusErr.StatusCode)))
	fmt.Fprintln(w, base.Render("Check "+endpointName+", and the instance's status if it persists."))

	return true
}

// hostOrEndpoint reduces an endpoint to scheme and host, falling back to the
// endpoint as given when it will not parse. Redacted either way.
func hostOrEndpoint(endpoint string) string {
	if host, err := config.SchemeHostOnly(endpoint); err == nil {
		return redacted(host)
	}

	return redacted(endpoint)
}

// redacted hides a password an endpoint carries in its userinfo. Fails closed: a
// value url.Parse rejects loses everything before the first `@`.
func redacted(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)

	parsed, err := url.Parse(trimmed)
	if err == nil {
		if parsed.User == nil {
			return endpoint
		}

		return parsed.Redacted()
	}

	if _, host, found := strings.Cut(trimmed, "@"); found {
		return "[redacted]@" + host
	}

	return endpoint
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

	base, info := writerStyles(w)

	fmt.Fprint(w, base.Render("Environment credentials for "))
	fmt.Fprint(w, info.Render(hostOrEndpoint(creds.Endpoint)))
	fmt.Fprint(w, base.Render(" failed to verify; not falling back to the stored profile for "))
	fmt.Fprint(w, info.Render(redacted(storedHost)))
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

	// Everything this gate says goes to stderr: it runs in PreRunE, and
	// --output-format json must leave stdout parseable.
	if creds.Token != "" {
		// A token with no endpoint was never verified; a login flow would shadow it.
		base, _ := writerStyles(os.Stderr)

		fmt.Fprintln(os.Stderr, base.Render("Your DATAROBOT_API_TOKEN environment variable is set"))
		fmt.Fprintln(os.Stderr, base.Render("without a DATAROBOT_ENDPOINT. Set that too, or unset the token:"))
		FprintUnsetTokenInstructions(os.Stderr)

		skipAuthFlow = true
	}

	// Only a stored token can be wrongly discarded, so only its failures suppress
	// the login flow. With nothing stored there is no verdict to respect.
	if viperx.GetString(config.DataRobotAPIKey) != "" &&
		ReportUnjudged(os.Stderr, viperx.GetString(config.DataRobotURL), StoredEndpointName, viperErr) {
		skipAuthFlow = true
	}

	if skipAuthFlow {
		return false
	}

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
