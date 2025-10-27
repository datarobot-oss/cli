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

## Manual login

You can still manually run `dr auth login` to refresh credentials or change accounts. The `LoginAction()` function provides the interactive login experience with confirmation prompts for overwriting existing credentials.
