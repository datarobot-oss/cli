# Authentication flow

## Overview

The CLI provides a reusable authentication mechanism that you can use with any command requiring valid DataRobot credentials. Cobra's `PreRunE` hooks handle authentication and ensure credentials are valid before a command executes.

## Use authentication in commands

### PreRunE hook

Use `auth.EnsureAuthenticatedE` as your command's `PreRunE` hook. It already has the
`func(*cobra.Command, []string) error` shape Cobra expects, so assign it directly.

```go
import "github.com/datarobot/cli/internal/auth"

var MyCmd = &cobra.Command{
    Use:     "mycommand",
    Short:   "My command description",
    PreRunE: auth.EnsureAuthenticatedE,
    Run: func(_ *cobra.Command, _ []string) {
        // Command implementation
        // Authentication is guaranteed to be valid here
    },
}
```

#### How it works

The hook functions are outlined below.

1. **Checks environment credentials first**: A complete `DATAROBOT_ENDPOINT` (or `DATAROBOT_API_ENDPOINT`) and `DATAROBOT_API_TOKEN` pair takes precedence over the config file. If the pair fails verification, the command fails with the reason (timeout, malformed endpoint, unreachable endpoint, a non-2xx status from the instance, or an invalid token; only a 401 or 403 blames the token). It never falls back to the stored profile and never starts the login flow, because that would silently run the command against a different DataRobot instance than the one requested.
2. **Checks for valid credentials**: With no complete environment pair, checks if a valid API key already exists in the config file.
3. **Auto-configures URL if missing**: If no DataRobot URL is configured, prompts you to set it up.
4. **Retrieves new credentials**: If the stored credentials are missing, or DataRobot rejected them with a 401 or 403, the hook automatically triggers the browser-based login flow. A timeout, an unreachable host, or any other status means DataRobot never judged the credentials, so the login flow does not start and the stored token is left intact.
5. **Fails early**: If authentication cannot be established, the command will not run and returns an error. Credential failures are explained on stderr, so `--output-format json` leaves stdout empty when the gate rejects your credentials. The interactive paths still use stdout: the browser login flow and the URL prompt.

### Direct call for non-command code

For code that isn't a Cobra command, call `auth.EnsureAuthenticated(ctx)` directly.

```go
import "github.com/datarobot/cli/internal/auth"

func MyFunction(ctx context.Context) error {
    // Ensure valid authentication before proceeding.
    if !auth.EnsureAuthenticated(ctx) {
        return errors.New("authentication failed")
    }

    // Continue with authenticated operations.
    apiKey, err := config.GetAPIKey(ctx)
    if err != nil {
        return err
    }
    // ... use apiKey for API calls

    return nil
}
```

#### When to use a direct call

Use a direct call to add authentication to any command that does the following:

- Makes API calls to DataRobot endpoints.
- Needs to populate DataRobot credentials in configuration files.
- Requires valid authentication to function correctly.

### Commands with authentication

The following commands use `PreRunE` to ensure authentication:

- `dr dotenv update`: Automatically ensures authentication before updating environment variables.
- `dr templates list`: Requires authentication to fetch templates from the API.
- `dr templates setup`: Requires authentication to fetch template details.

## Skip authentication

For advanced use cases where authentication is handled externally or not required, you can bypass authentication checks using the `--skip-auth` global flag.

Use the `--skip-auth` global flag to skip authentication for any command. You can also skip authentication with an environment variable.

```bash
# Skip authentication for any command
dr templates list --skip-auth
dr dotenv update --skip-auth

# Skip authentication with environment variable
DATAROBOT_CLI_SKIP_AUTH=true dr templates setup
```

#### Behavior

When `--skip-auth` is enabled, it expect the following behavior:

1. **Bypass all authentication checks**: The `EnsureAuthenticated()` function returns `true` immediately without validating credentials.
2. **Emit a warning**: Logs a warning message: `Authentication checks are disabled via --skip-auth flag. This may cause API calls to fail`.
3. **May cause API failures**: Commands that make API calls will likely fail if no valid credentials are present.

#### When to use skip-auth

The `--skip-auth` flag is intended for advanced scenarios such as:

- **Testing**: Test command logic without requiring valid credentials.
- **CI/CD pipelines**: Use when authentication is managed through environment variables (`DATAROBOT_API_TOKEN`).
- **Offline development**: When working in environments without internet access or access to DataRobot.
- **Debugging**: Isolate authentication issues from other command behavior.

> [!WARNING]
> The `--skip-auth` flag should only be used when you understand the implications. Most users should rely on the standard authentication flow via `dr auth login`.

## Manual login

You can still run `dr auth login` to refresh credentials or change accounts. Both that
command and the implicit login inside `EnsureAuthenticated` go through
`auth.RunBrowserLogin`, so the two present identically.

### Browser login internals

`auth.BrowserFlow` (`internal/auth/browserflow.go`) owns the callback listener and splits
the login into three steps, which is what lets the blocking CLI path and the bubbletea
setup wizard share one implementation:

```go
flow, err := auth.NewBrowserFlow(host) // binds localhost:51164
defer flow.Close()

openErr := flow.OpenBrowser()          // best effort; report the link if non-nil
key, err := flow.Wait(ctx)             // serves until the callback arrives
```

Three rules matter when changing this code:

- **Bind before opening the browser.** `NewBrowserFlow` binds the listener so a fast
  redirect cannot arrive before anything is listening.
- **Never claim the browser opened without checking.** `open.Open` returns an error;
  pass it through `auth.BrowserStateFor` and let `auth.RenderBrowserPrompt` choose the
  wording. Telling users to watch for a browser that never opened is the bug CFX-6318
  fixed.
- **Keep the callback's `Sec-Fetch-Dest` gate present-and-wrong, never absent.**
  `handleCallback` refuses a request whose `Sec-Fetch-Dest` is set to anything but
  `document`, which turns away a page's `<img>` or `fetch` without touching the real
  callback (a top-level navigation). An absent header must still be accepted: the
  CLI-to-CLI port handover in `listenReclaimingPort` uses a Go `http.Client`, which sends
  no fetch metadata, so rejecting absent would deadlock two concurrent logins. Do not
  gate on `Sec-Fetch-Site`: the genuine callback is legitimately cross-site.

`auth.RunBrowserLoginWith` accepts `LoginOptions{NoBrowser: true}` for `--no-browser`,
which renders the link prominently without reporting a failure.

## Internal APIs

The auth package writes configuration through the allowlisted writer in
`internal/config` (`config.UpdateConfigFile`). It does **not** call
`viper.WriteConfig()` directly, because that would serialize every key in
`viper.AllSettings()`&mdash;including transient flags such as `--yes`&mdash;
into `drconfig.yaml`. Outside `internal/config/...`, all viper access
goes through the `internal/config/viperx` wrapper. See
[Configuration](configuration.md) for the full contract.

`drconfig.yaml` holds the API token in plaintext, so `config.UpdateConfigFile` writes it
with mode `0600` **and** chmods it on every write. The chmod is not redundant:
`os.WriteFile` only applies its perm argument when it creates the file, so a config left
at `0644` by an older CLI would otherwise stay world-readable forever.

- `WriteConfigFileSilent()`&mdash;writes only allowlisted keys
  (`config.PersistableKeys`) back to `drconfig.yaml` and returns an error.
- `WriteConfigFile()`&mdash;writes the config file, prints a success
  message, and returns an error.
- `SetURLAction()`&mdash;asks for a DataRobot URL and returns whether it changed. On an
  interactive terminal it runs the `tui/hostpicker` list; otherwise it falls back to a
  plain-text stdin prompt so piped input and the expect-based smoke tests keep working.
- `config.SkipAuthKey`&mdash;the viper key for `--skip-auth`. Use the constant at every
  bind and read site: viper does not treat `-` and `_` as equivalent for lookups, and
  reading `"skip_auth"` silently ignored the bound flag.
