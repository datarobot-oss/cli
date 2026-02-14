# Shell completions

The DataRobot CLI supports auto-completion for Bash, Zsh, Fish, and PowerShell. Shell completions provide:

- Command and subcommand suggestions.
- Flag and option completions.
- Faster command entry via tab completion.
- Discovery of available commands.

## Installation

### Automatic installation

You can use three different methods to install shell completions:

1. **Installation script**: Recommended for first-time installs.

    The installer automatically detects your shell and configures completions.

   ```bash
   curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | sh
   ```

2. **Interactive command**: Recommended for managing completions.

    This command detects your shell and installs completions to the appropriate location.

   ```bash
   dr self completion install
   ```

3. **Manual installation**: Recommended for advanced users. Follow the shell-specific instructions below.

### Interactive commands

The CLI provides commands to easily manage completions.

```bash
# Install completions for your current shell
dr self completion install

# Force reinstall (useful after updates)
dr self completion install --force

# Uninstall completions
dr self completion uninstall
```

### Manual installation

If you prefer manual installation or the automatic methods do not work, follow the instructions below.

### Bash

#### Linux

```bash
# Generate and install the completion script
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Reload your shell
source ~/.bashrc
```

```zsh
# If shell completion is not already enabled in your environment you will need
# to enable it.  You can execute the following command once:
# echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute the following command once:
  $ dr self completion zsh > "${fpath[1]}/_dr"
```

#### macOS

The default shell in macOS is `zsh`. Shell completions for `zsh`
are typically stored in one of the following directories:

- `/usr/local/share/zsh/site-functions/`
- `/opt/homebrew/share/zsh/site-functions/`
- `${ZDOTDIR:-$HOME}/.zsh/completions/`

Run `echo $fpath` to see all possibilities. For example, if you
wish to put CLI completions into ZDOTDIR, then run:

```zsh
dr self completion zsh > ${ZDOTDIR:-$HOME}/.zsh/completions/_dr
```

If you use Bash on macOS, Homebrew stores bash completions in its prefix. If completion does not work after install, check that your shell loads files from:

- `$(brew --prefix)/etc/bash_completion.d`.

#### Temporary session

For the current session only:

```bash
source <(dr self completion bash)
```

### Zsh

#### Setup

First, ensure completion is enabled in your `~/.zshrc`:

```bash
# Add these lines if not already present
autoload -U compinit
compinit
```

#### Installation

```bash
# Create completions directory if it doesn't exist
mkdir -p ~/.zsh/completions

# Generate completion script
dr self completion zsh > ~/.zsh/completions/_dr

# Add to fpath in ~/.zshrc (if not already there)
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc

# Reload your shell
source ~/.zshrc
```

#### Alternative (using system directory)

```bash
# Generate and install the completion script
dr self completion zsh > "${fpath[1]}/_dr"

# Clear completion cache
rm -f ~/.zcompdump

# Reload your shell
source ~/.zshrc
```

#### Temporary session

For the current session only:

```bash
source <(dr self completion zsh)
```

### Fish

```bash
# Generate and install the completion script
dr self completion fish > ~/.config/fish/completions/dr.fish

# Reload fish configuration
source ~/.config/fish/config.fish
```

#### Temporary session

For the current session only:

```bash
dr self completion fish | source
```

### PowerShell

#### Persistent installation

To add the CLI to your PowerShell profile:

```powershell
# Generate the completion script
dr self completion powershell > dr.ps1

# Find your profile location
echo $PROFILE

# Add the following line to your profile
. C:\path\to\dr.ps1
```

Alternatively, you can install it directly:

```powershell
# Add to profile
dr self completion powershell >> $PROFILE

# Reload profile
. $PROFILE
```

#### Temporary session

For the current session only:

```powershell
dr self completion powershell | Out-String | Invoke-Expression
```

## Usage

Once installed, completions work automatically when you press `Tab`:

### Command completion

```bash
# Type 'dr' and press Tab to see all commands
dr <Tab>
# Shows: auth, completion, dotenv, run, templates, version

# Type 'dr auth' and press Tab to see subcommands
dr auth <Tab>
# Shows: check, login, logout, set-url

# Type 'dr templates' and press Tab
dr templates <Tab>
# Shows: clone, list, setup, status
```

### Flag completion

```bash
# Type a command and -- then Tab to see flags
dr run --<Tab>
# Shows: --concurrency, --dir, --exit-code, --help, --list, --parallel, --silent, --watch, --yes

# Partial flag matching works too
dr run --par<Tab>
# Completes to: dr run --parallel
```

### Argument completion

For commands that support it:

```bash
# Template names when using setup
dr templates setup <Tab>
# Shows available template names from DataRobot

# Task names when using run (if in a template directory)
dr run <Tab>
# Shows available tasks from Taskfile
```

## Verification

Test that completions are working:

```bash
# Try command completion
dr te<Tab>
# Should complete to: dr templates

# Try flag completion
dr run --l<Tab>
# Should complete to: dr run --list
```

## Troubleshooting

### Completions not working

#### Bash

**Important**: Bash completions require the `bash-completion` package to be installed first.

1. Install bash-completion if not already installed:

   ```bash
   # macOS (Homebrew)
   brew install bash-completion@2

   # Ubuntu/Debian
   sudo apt-get install bash-completion

   # RHEL/CentOS
   sudo yum install bash-completion
   ```

2. Check that bash-completion is loaded:

   ```bash
   # macOS
   brew list bash-completion@2

   # Linux (Ubuntu/Debian)
   dpkg -l | grep bash-completion
   ```

3. Verify completion script location:

   ```bash
   ls -l /etc/bash_completion.d/dr
   # or on macOS
   ls -l $(brew --prefix)/etc/bash_completion.d/dr
   ```

4. Check `.bashrc` sources completion:

   ```bash
   grep bash_completion ~/.bashrc
   ```

5. Reload your shell:
   ```bash
   source ~/.bashrc
   ```

#### Zsh

1. Verify `compinit` is in `~/.zshrc`:
   ```bash
   grep compinit ~/.zshrc
   ```

2. Check completion file location:

   ```bash
   ls -l ~/.zsh/completions/_dr
   # or
   echo $fpath[1]
   ls -l $fpath[1]/_dr
   ```

3. Clear completion cache:
   ```bash
   rm -f ~/.zcompdump
   ```

4. Reload Zsh:
   ```bash
   exec zsh
   ```

#### Fish

1. Check that the completion file exists:
   ```bash
   ls -l ~/.config/fish/completions/dr.fish
   ```

2. Verify that Fish recognizes it:
   ```bash
   complete -C dr
   ```

3. Reload Fish:
   ```bash
   source ~/.config/fish/config.fish
   ```

#### PowerShell

1. Check execution policy:
   ```powershell
   Get-ExecutionPolicy
   ```

   If it's `Restricted`, change it:
   ```powershell
   Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
   ```

2. Verify profile loads completion:
   ```powershell
   cat $PROFILE
   ```

3. Reload profile:
   ```powershell
   . $PROFILE
   ```

### Permission denied

If you get permission errors when installing:

```bash
# Use sudo for system-wide installation
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Or use user-level installation
dr self completion bash > ~/.bash_completions/dr
source ~/.bash_completions/dr
```

### Completion cache issues

For Zsh, if completions are outdated:

```bash
# Clear cache
rm -f ~/.zcompdump*

# Rebuild cache
compinit
```

## Advanced configuration

### Custom completion behavior

You can customize how completions work by modifying the generated script.

For example, in the Bash completion script, you can add custom completion logic:

```bash
dr self completion bash > ~/dr-completion.bash
vim ~/dr-completion.bash

# Source it in your .bashrc
source ~/dr-completion.bash
```

### Multiple shell support

If you use multiple shells, install completions for each:

```bash
dr self completion bash > ~/.bash_completions/dr
dr self completion zsh > ~/.zsh/completions/_dr
dr self completion fish > ~/.config/fish/completions/dr.fish
```

## Updating completions

When the CLI is updated, regenerate completions:

```bash
# Bash
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr self completion zsh > ~/.zsh/completions/_dr
rm -f ~/.zcompdump

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish

# PowerShell
dr self completion powershell >> $PROFILE
```

## See also

- [Quick start](../../README.md#quick-start)&mdash;get started with the CLI.
- [Command reference](../commands/)&mdash;browse commands and flags.
- [Cobra documentation](https://github.com/spf13/cobra/blob/main/shell_completions.md)&mdash;completion implementation details.
