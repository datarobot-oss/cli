# Shell Completions

The DataRobot CLI supports auto-completion for Bash, Zsh, Fish, and PowerShell. Shell completions provide:

- Command and subcommand suggestions
- Flag and option completions
- Faster command entry with tab completion
- Discovery of available commands

## Installation

### Bash

#### Linux

```bash
# Generate and install completion script
dr completion bash | sudo tee /etc/bash_completion.d/dr

# Reload your shell
source ~/.bashrc
```

#### macOS

First, ensure bash-completion is installed:

```bash
# Using Homebrew
brew install bash-completion@2
```

Then install the completion script:

```bash
# Generate and install completion script
dr completion bash > $(brew --prefix)/etc/bash_completion.d/dr

# Reload your shell
source ~/.bash_profile
```

#### Temporary Session

For the current session only:

```bash
source <(dr completion bash)
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
dr completion zsh > ~/.zsh/completions/_dr

# Add to fpath in ~/.zshrc (if not already there)
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc

# Reload your shell
source ~/.zshrc
```

#### Alternative (using system directory)

```bash
# Generate and install completion script
dr completion zsh > "${fpath[1]}/_dr"

# Clear completion cache
rm -f ~/.zcompdump

# Reload your shell
source ~/.zshrc
```

#### Temporary Session

For the current session only:

```bash
source <(dr completion zsh)
```

### Fish

```bash
# Generate and install completion script
dr completion fish > ~/.config/fish/completions/dr.fish

# Reload fish configuration
source ~/.config/fish/config.fish
```

#### Temporary Session

For the current session only:

```bash
dr completion fish | source
```

### PowerShell

#### Persistent Installation

Add to your PowerShell profile:

```powershell
# Generate completion script
dr completion powershell > dr.ps1

# Find your profile location
echo $PROFILE

# Add this line to your profile
. C:\path\to\dr.ps1
```

Or install directly:

```powershell
# Add to profile
dr completion powershell >> $PROFILE

# Reload profile
. $PROFILE
```

#### Temporary Session

For the current session only:

```powershell
dr completion powershell | Out-String | Invoke-Expression
```

## Usage

Once installed, completions work automatically when you press `Tab`:

### Command Completion

```bash
# Type 'dr' and press Tab to see all commands
dr <Tab>
# Shows: auth, completion, dotenv, run, templates, version

# Type 'dr auth' and press Tab to see subcommands
dr auth <Tab>
# Shows: login, logout, set-url

# Type 'dr templates' and press Tab
dr templates <Tab>
# Shows: clone, list, setup, status
```

### Flag Completion

```bash
# Type a command and -- then Tab to see flags
dr run --<Tab>
# Shows: --concurrency, --dir, --exit-code, --help, --list, --parallel, --silent, --watch, --yes

# Partial flag matching works too
dr run --par<Tab>
# Completes to: dr run --parallel
```

### Argument Completion

For commands that support it:

```bash
# Template names when using clone
dr templates clone <Tab>
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

### Completions Not Working

#### Bash

1. Check that bash-completion is installed:
   ```bash
   # macOS
   brew list bash-completion@2
   
   # Linux (Ubuntu/Debian)
   dpkg -l | grep bash-completion
   ```

2. Verify completion script location:
   ```bash
   ls -l /etc/bash_completion.d/dr
   # or on macOS
   ls -l $(brew --prefix)/etc/bash_completion.d/dr
   ```

3. Check `.bashrc` sources completion:
   ```bash
   grep bash_completion ~/.bashrc
   ```

4. Reload your shell:
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

1. Check completion file exists:
   ```bash
   ls -l ~/.config/fish/completions/dr.fish
   ```

2. Verify Fish recognizes it:
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

### Permission Denied

If you get permission errors when installing:

```bash
# Use sudo for system-wide installation
sudo dr completion bash > /etc/bash_completion.d/dr

# Or use user-level installation
dr completion bash > ~/.bash_completions/dr
source ~/.bash_completions/dr
```

### Completion Cache Issues

For Zsh, if completions are outdated:

```bash
# Clear cache
rm -f ~/.zcompdump*

# Rebuild cache
compinit
```

## Advanced Configuration

### Custom Completion Behavior

You can customize how completions work by modifying the generated script.

For example, in Bash completion script, you can add custom completion logic:

```bash
# Extract the generated script
dr completion bash > ~/dr-completion.bash

# Edit the script to add custom logic
vim ~/dr-completion.bash

# Source it in your .bashrc
source ~/dr-completion.bash
```

### Multiple Shell Support

If you use multiple shells, install completions for each:

```bash
# Install for all shells you use
dr completion bash > ~/.bash_completions/dr
dr completion zsh > ~/.zsh/completions/_dr
dr completion fish > ~/.config/fish/completions/dr.fish
```

## Updating Completions

When the CLI is updated, regenerate completions:

```bash
# Bash
dr completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr completion zsh > ~/.zsh/completions/_dr
rm -f ~/.zcompdump

# Fish
dr completion fish > ~/.config/fish/completions/dr.fish

# PowerShell
dr completion powershell > $PROFILE
```

## See Also

- [Getting Started](getting-started.md) - Initial setup guide
- [Command Reference](../commands/) - Complete command documentation
- [Cobra Documentation](https://github.com/spf13/cobra/blob/main/shell_completions.md) - Underlying completion framework
