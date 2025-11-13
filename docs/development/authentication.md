# Authentication flow

## Overview

The CLI provides a reusable authentication mechanism that you can use with any command that requires valid DataRobot credentials. Authentication is handled using Cobra's `PreRunE` hooks, which ensure credentials are valid before a command executes.

## Using authentication in commands

### PreRunE hook (recommended)

The recommended approach is to use the `auth.EnsureAuthenticatedE()` function in your command's `PreRunE` hook:

```go
import "github.com/datarobot/cli/cmd/auth"

var MyCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "My command description",
    PreRunE: func(_ *cobra.Command, _ []string) error {
        return auth.EnsureAuthenticatedE()
    },
    Run: func(_ *cobra.Command, _ []string) {
        // Command implementation
        // Authentication is guaranteed to be valid here
    },
}
```

### How it works

1. **Checks for valid credentials**&mdash;first checks if a valid API key already exists.
2. **Auto-configures URL if missing**&mdash;if no DataRobot URL is configured, prompts you to set it up.
3. **Retrieves new credentials**&mdash;if credentials are missing or expired, automatically triggers the browser-based login flow.
4. **Fails early**&mdash;if authentication cannot be established, the command won't run and returns an error.

### Direct call (for non-command code)

For code that isn't a Cobra command, you can use `auth.EnsureAuthenticated()` directly:

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

### When to use

Add authentication to any command that:

- Makes API calls to DataRobot endpoints.
- Needs to populate DataRobot credentials in configuration files.
- Requires valid authentication to function correctly.

### Commands with authentication

The following commands use `PreRunE` to ensure authentication:

- **`dr dotenv update`**&mdash;automatically ensures authentication before updating environment variables.
- **`dr templates list`**&mdash;requires authentication to fetch templates from the API.
- **`dr templates clone`**&mdash;requires authentication to fetch template details.

## Skipping authentication

For advanced use cases where authentication is handled externally or not required, you can bypass authentication checks using the `--skip-auth` global flag.

### Using the skip-auth flag

```bash
# Skip authentication for any command
dr templates list --skip-auth
dr dotenv update --skip-auth

# Skip authentication with environment variable
DATAROBOT_CLI_SKIP_AUTH=true dr templates setup
```

### Behavior

When `--skip-auth` is enabled:

1. **Bypasses all authentication checks**&mdash;the `EnsureAuthenticated()` function returns `true` immediately without validating credentials.
2. **Emits a warning**&mdash;logs a warning message: "Authentication checks are disabled via --skip-auth flag. This may cause API calls to fail."
3. **May cause API failures**&mdash;commands that make API calls will likely fail if no valid credentials are present.

### When to use skip-auth

The `--skip-auth` flag is intended for advanced scenarios such as:

- **Testing**&mdash;testing command logic without requiring valid credentials.
- **CI/CD pipelines**&mdash;when authentication is managed through environment variables (`DATAROBOT_API_TOKEN`).
- **Offline development**&mdash;working in environments without internet access or access to DataRobot.
- **Debugging**&mdash;isolating authentication issues from other command behavior.

> **⚠️ Warning:** This flag should only be used when you understand the implications. Most users should rely on the standard authentication flow via `dr auth login`.

## Manual login

You can still manually run `dr auth login` to refresh credentials or change accounts. The `LoginAction()` function provides the interactive login experience with confirmation prompts for overwriting existing credentials.
