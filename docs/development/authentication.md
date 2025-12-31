# Authentication flow

## Overview

The CLI provides a reusable authentication mechanism that you can use with any command requiring valid DataRobot credentials. Cobra's `PreRunE` hooks handle authentication and ensure credentials are valid before a command executes.

## Use authentication in commands

### PreRunE hook

Use the `auth.EnsureAuthenticatedE(ctx)` function in your command's `PreRunE` hook.

```go
import "github.com/datarobot/cli/cmd/auth"

var MyCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "My command description",
    PreRunE: func(cmd *cobra.Command, _ []string) error {
        return auth.EnsureAuthenticatedE(cmd.Context())
    },
    Run: func(_ *cobra.Command, _ []string) {
        // Command implementation
        // Authentication is guaranteed to be valid here
    },
}
```

#### How it works

The hook functions are outlined below.

1. **Checks for valid credentials**: Checks if a valid API key already exists.
2. **Auto-configures URL if missing**: If no DataRobot URL is configured, prompts you to set it up.
3. **Retrieves new credentials**: If credentials are missing or expired, the hook automatically triggers the browser-based login flow.
4. **Fails early**: If authentication cannot be established, the command will not run and  returns an error.

### Direct call for non-command code

For code that isn't a Cobra command, you can use `auth.EnsureAuthenticated()` directly.

```go
import "github.com/datarobot/cli/cmd/auth"

func MyFunction() error {
    // Ensure valid authentication before proceeding.
    if !auth.EnsureAuthenticated() {
        return errors.New("authentication failed")
    }

    // Continue with authenticated operations.
    apiKey := config.GetAPIKey()
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
- `dr templates clone`: Requires authentication to fetch template details.

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

You can still manually run `dr auth login` to refresh credentials or change accounts. The `LoginAction()` function provides the interactive login experience with confirmation prompts for overwriting existing credentials.

## Internal APIs

The auth package writes configuration through Viper.

- `WriteConfigFileSilent()`&mdash;writes the config file and returns an error.
- `WriteConfigFile()`&mdash;writes the config file, prints a success message, and returns an error.
- `SetURLAction()`&mdash;prompts for a DataRobot URL, optionally overwrites an existing value, and returns a boolean indicating whether the URL changed.
