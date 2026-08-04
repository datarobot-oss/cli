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

// Package export renders the DataRobot credentials the CLI is currently using
// as shell statements that set the canonical DataRobot environment variables,
// so they can be sourced into the caller's shell session.
package export

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/outputformat"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// The canonical DataRobot environment variables. Every DataRobot SDK, the
// platform itself, and this CLI read this pair, so they are what we emit.
const (
	endpointVar = "DATAROBOT_ENDPOINT"
	tokenVar    = "DATAROBOT_API_TOKEN"
)

// jsonEnvelopeKey is the top-level key used for `--output-format json`.
const jsonEnvelopeKey = "environment"

var (
	errNoCredentials = errors.New("no DataRobot credentials configured")
	errNoEndpoint    = errors.New("no DataRobot endpoint configured")
	errNoToken       = errors.New("no DataRobot API token configured")
)

// credentials is the endpoint/token pair this command renders.
type credentials struct {
	Endpoint string
	Token    string
}

// statementFunc renders a single "set this variable" statement for one shell.
type statementFunc func(name, value string) string

// canonicalEndpoint normalizes a configured DataRobot URL into the canonical
// API endpoint form, e.g. "app.datarobot.com" becomes
// "https://app.datarobot.com/api/v2". A URL that already carries a path is
// preserved as-is (minus any trailing slash, query, or fragment) so that
// self-managed installs serving the API under a custom prefix keep working.
func canonicalEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errNoEndpoint
	}

	// Default to https so a bare hostname is accepted, matching
	// config.SchemeHostOnly.
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}

	if parsed.Host == "" {
		return "", config.ErrInvalidURL
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = config.DRAPIURLSuffix
	}

	parsed.Path, parsed.RawQuery, parsed.Fragment = path, "", ""

	return parsed.String(), nil
}

// rawCredentials returns the credentials the CLI would use, mirroring the
// precedence in auth.EnsureAuthenticated: a complete pair of environment
// variables wins over the CLI config file. Unlike EnsureAuthenticated, it never
// contacts the API and never starts a login flow, so `dr auth export` stays
// usable from shell startup files and non-interactive sessions.
func rawCredentials() (credentials, error) {
	if env := auth.GetEnvCredentials(); env.Endpoint != "" && env.Token != "" {
		return credentials{Endpoint: env.Endpoint, Token: env.Token}, nil
	}

	creds := credentials{
		Endpoint: viperx.GetString(config.DataRobotURL),
		Token:    viperx.GetString(config.DataRobotAPIKey),
	}

	switch {
	case creds.Endpoint == "" && creds.Token == "":
		return creds, errNoCredentials
	case creds.Endpoint == "":
		return creds, errNoEndpoint
	case creds.Token == "":
		return creds, errNoToken
	}

	return creds, nil
}

// resolveCredentials returns the credentials to export, with the endpoint
// normalized to its canonical form.
func resolveCredentials() (credentials, error) {
	creds, err := rawCredentials()
	if err != nil {
		return creds, err
	}

	endpoint, err := canonicalEndpoint(creds.Endpoint)
	if err != nil {
		return creds, err
	}

	creds.Endpoint = endpoint

	return creds, nil
}

// printCredentialError explains on stderr why nothing was exported. stdout is
// left empty so that `eval "$(dr auth export)"` cannot evaluate a partial or
// non-executable result.
func printCredentialError(w io.Writer, err error) {
	switch {
	case errors.Is(err, errNoEndpoint):
		fmt.Fprintln(w, tui.BaseTextStyle.Render("❌ No DataRobot URL configured."))
		fmt.Fprint(w, tui.BaseTextStyle.Render("Run "))
		fmt.Fprint(w, tui.InfoStyle.Render(version.CliName+" auth set-url"))
		fmt.Fprintln(w, tui.BaseTextStyle.Render(" to configure your DataRobot URL."))
	case errors.Is(err, errNoToken), errors.Is(err, errNoCredentials):
		fmt.Fprintln(w, tui.BaseTextStyle.Render("❌ No DataRobot credentials found."))
		fmt.Fprint(w, tui.BaseTextStyle.Render("Run "))
		fmt.Fprint(w, tui.InfoStyle.Render(version.CliName+" auth login"))
		fmt.Fprintln(w, tui.BaseTextStyle.Render(" to authenticate."))
	default:
		fmt.Fprintln(w, tui.BaseTextStyle.Render("❌ Invalid DataRobot URL configured: "+err.Error()))
		fmt.Fprint(w, tui.BaseTextStyle.Render("Run "))
		fmt.Fprint(w, tui.InfoStyle.Render(version.CliName+" auth set-url"))
		fmt.Fprintln(w, tui.BaseTextStyle.Render(" to fix it."))
	}
}

// posixQuote wraps a value in single quotes and escapes any embedded single
// quote, producing one literal word in sh, bash, zsh, and fish.
func posixQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// powerShellQuote wraps a value in single quotes, where an embedded quote is
// escaped by doubling it.
func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// cmdEscape caret-escapes the cmd.exe metacharacters. `set NAME=value` has no
// quoting form — surrounding quotes would become part of the value — so the
// metacharacters have to be escaped individually.
func cmdEscape(value string) string {
	return strings.NewReplacer(
		"^", "^^",
		"&", "^&",
		"<", "^<",
		">", "^>",
		"|", "^|",
	).Replace(value)
}

func posixStatement(name, value string) string {
	return "export " + name + "=" + posixQuote(value)
}

func fishStatement(name, value string) string {
	return "set -gx " + name + " " + posixQuote(value)
}

func powerShellStatement(name, value string) string {
	return "$env:" + name + " = " + powerShellQuote(value)
}

func cmdStatement(name, value string) string {
	return "set " + name + "=" + cmdEscape(value)
}

// renderExports returns the statements that set both canonical variables in the
// syntax of the given shell.
func renderExports(sh internalShell.Shell, creds credentials) []string {
	var statement statementFunc

	switch sh {
	case internalShell.Fish:
		statement = fishStatement
	case internalShell.PowerShell:
		statement = powerShellStatement
	case internalShell.Cmd:
		statement = cmdStatement
	case internalShell.Bash, internalShell.Zsh:
		statement = posixStatement
	default:
		// Unrecognized shells (e.g. plain sh, dash, ksh) get POSIX syntax,
		// which is correct for every Bourne-compatible shell.
		statement = posixStatement
	}

	return []string{
		statement(endpointVar, creds.Endpoint),
		statement(tokenVar, creds.Token),
	}
}

// resolveShell picks the syntax to emit. An explicit --shell wins; otherwise
// the parent shell is detected. Detection failures fall back to POSIX syntax
// instead of erroring, since that is the right answer for sh, bash, and zsh.
func resolveShell(requested string) (internalShell.Shell, error) {
	if requested != "" {
		normalized := strings.ToLower(strings.TrimSpace(requested))
		if !slices.Contains(internalShell.SupportedShells(), normalized) {
			return "", fmt.Errorf("Unsupported shell %q: use one of %s.",
				requested, strings.Join(internalShell.SupportedShells(), ", "))
		}

		return internalShell.Shell(normalized), nil
	}

	detected, err := internalShell.DetectShell()
	if err != nil {
		log.Debug("auth export: shell detection failed, falling back to POSIX syntax",
			"error", err, "shell", internalShell.Bash)

		return internalShell.Bash, nil
	}

	log.Debug("auth export: detected shell", "shell", detected)

	return internalShell.Shell(detected), nil
}

func RunE(cmd *cobra.Command, _ []string) error {
	shellFlag, err := cmd.Flags().GetString("shell")
	if err != nil {
		return err
	}

	sh, err := resolveShell(shellFlag)
	if err != nil {
		return err
	}

	creds, err := resolveCredentials()
	if err != nil {
		printCredentialError(cmd.ErrOrStderr(), err)

		return cli.ErrSilent
	}

	if outputformat.GetFormat(cmd) == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), jsonEnvelopeKey, map[string]string{
			endpointVar: creds.Endpoint,
			tokenVar:    creds.Token,
		})
	}

	for _, statement := range renderExports(sh, creds) {
		fmt.Fprintln(cmd.OutOrStdout(), statement)
	}

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "export",
		Short:         "📤 Print shell statements that set the DataRobot environment variables",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `Print the canonical DataRobot environment variables for the credentials the
CLI is currently using:

  ` + endpointVar + `=https://<your-install>/api/v2
  ` + tokenVar + `=<token>

Only the shell statements are written to stdout, so the output can be evaluated
directly by your shell. Errors and hints go to stderr.

Credentials are resolved the same way the rest of the CLI resolves them:
  • the ` + endpointVar + ` and ` + tokenVar + ` environment variables, if both are set
  • otherwise the CLI config file written by '` + version.CliName + ` auth login'

A project's .env file is never consulted — this exports the CLI's own
credentials. Use '` + version.CliName + ` dotenv setup' to manage .env files.

Unlike most commands, this one never starts a login flow and never calls the
API, so it is safe to run from a shell startup file. Run '` + version.CliName + ` auth check' first
if you want the credentials validated.

The syntax matches the detected parent shell; override it with --shell.

⚠️  The output contains your API token in plain text. Avoid piping it into a
    shared terminal, a log, or a file that is checked into version control.`,
		Example: `  # bash / zsh
  eval "$(` + version.CliName + ` auth export)"

  # fish
  ` + version.CliName + ` auth export | source

  # PowerShell
  ` + version.CliName + ` auth export | Out-String | Invoke-Expression

  # cmd.exe
  for /f "usebackq delims=" %i in (` + "`" + version.CliName + ` auth export --shell cmd` + "`" + `) do @%i

  # save for later sourcing
  ` + version.CliName + ` auth export --shell bash > ~/.datarobot-env && source ~/.datarobot-env

  # machine-readable
  ` + version.CliName + ` auth export --output-format json`,
		RunE: RunE,
	}

	cmd.Flags().String("shell", "",
		fmt.Sprintf("Shell syntax to emit (%s); defaults to the detected shell",
			strings.Join(internalShell.SupportedShells(), ", ")))

	_ = cmd.RegisterFlagCompletionFunc("shell", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return internalShell.SupportedShells(), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}
