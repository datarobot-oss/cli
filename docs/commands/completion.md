# `dr self completion` - Shell completion

Generate shell completion scripts for command auto-completion.

## Quick start

For most users, setting up shell completion is a single command:

```bash
# Generate and install completion for your shell
# Bash (Linux)
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr self completion zsh > "${fpath[1]}/_dr"

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish
```

After installation, reload your shell to activate completions.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr self completion <shell>
```

## Description

The `self completion` command generates shell completion scripts that enable auto-completion for the DataRobot CLI. Completions provide command, subcommand, and flag suggestions when you press Tab.

## Supported shells

- `bash`&mdash;Bourne Again Shell.
- `zsh`&mdash;Z Shell.
- `fish`&mdash;Friendly Interactive Shell.
- `powershell`&mdash;PowerShell.

## Usage

### Bash

**Linux:**

```bash
# Install system-wide
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Reload shell
source ~/.bashrc
```

**macOS:**

```bash
# Install via Homebrew's bash-completion
brew install bash-completion@2
dr self completion bash > $(brew --prefix)/etc/bash_completion.d/dr

# Reload shell
source ~/.bash_profile
```

**Temporary (current session only):**

```bash
source <(dr self completion bash)
```

### Zsh

**Setup:**

First, ensure completion is enabled:

```bash
# Add to ~/.zshrc if not present
autoload -U compinit
compinit
```

**Installation:**

```bash
# Option 1: User completions directory
mkdir -p ~/.zsh/completions
dr self completion zsh > ~/.zsh/completions/_dr
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc

# Option 2: System directory
dr self completion zsh > "${fpath[1]}/_dr"

# Clear cache and reload
rm -f ~/.zcompdump
source ~/.zshrc
```

**Temporary (current session only):**

```bash
source <(dr self completion zsh)
```

### Fish

```bash
# Install completion
dr self completion fish > ~/.config/fish/completions/dr.fish

# Reload Fish
source ~/.config/fish/config.fish
```

**Temporary (current session only):**

```bash
dr self completion fish | source
```

### PowerShell

**Persistent:**

```powershell
# Generate completion script
dr self completion powershell > dr.ps1

# Add to PowerShell profile
Add-Content $PROFILE ". C:\path\to\dr.ps1"

# Reload profile
. $PROFILE
```

**Temporary (current session only):**

```powershell
dr self completion powershell | Out-String | Invoke-Expression
```

## Examples

### Generate completion script

```bash
# View the generated script
dr self completion bash

# Save to a file
dr self completion bash > dr-completion.bash

# Save for all shells
dr self completion bash > dr-completion.bash
dr self completion zsh > dr-completion.zsh
dr self completion fish > dr-completion.fish
dr self completion powershell > dr-completion.ps1
```

### Install for multiple shells

If you use multiple shells:

```bash
# Bash
dr self completion bash > ~/.bash_completions/dr

# Zsh
dr self completion zsh > ~/.zsh/completions/_dr

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish
```

### Update completions

After updating the CLI:

```bash
# Bash
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr self completion zsh > ~/.zsh/completions/_dr
rm -f ~/.zcompdump
exec zsh

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish
```

## Completion behavior

### Command completion

```bash
$ dr <Tab>
auth       completion dotenv     run        templates  version

$ dr auth <Tab>
login      logout     set-url

$ dr templates <Tab>
clone      list       setup      status
```

### Flag completion

```bash
$ dr run --<Tab>
--concurrency  --dir         --exit-code   --help
--list         --parallel    --silent      --watch
--yes

$ dr --<Tab>
--debug    --help     --verbose
```

### Argument completion

Some commands support argument completion:

```bash
# Task names (when in a template directory)
$ dr run <Tab>
build  dev  deploy  lint  test
```

## Troubleshooting

### Completions not working

**Bash:**

1. Verify bash-completion is installed:

   ```bash
   # macOS
   brew list bash-completion@2

   # Linux
   dpkg -l | grep bash-completion
   ```

2. Check if completion script exists:

   ```bash
   ls -l /etc/bash_completion.d/dr
   ```

3. Ensure .bashrc sources completions:

   ```bash
   grep bash_completion ~/.bashrc
   ```

4. Reload shell:

   ```bash
   source ~/.bashrc
   ```

**Zsh:**

1. Verify compinit is called:

   ```bash
   grep compinit ~/.zshrc
   ```

2. Check fpath includes completion directory:

   ```bash
   echo $fpath
   ```

3. Clear completion cache:

   ```bash
   rm -f ~/.zcompdump*
   compinit
   ```

4. Reload shell:

   ```bash
   exec zsh
   ```

**Fish:**

1. Check completion file:

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

**PowerShell:**

1. Check execution policy:

   ```powershell
   Get-ExecutionPolicy
   ```

   If restricted:

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

Use user-level installation instead of system-wide:

```bash
# Bash - user level
mkdir -p ~/.bash_completions
dr self completion bash > ~/.bash_completions/dr
echo 'source ~/.bash_completions/dr' >> ~/.bashrc

# Zsh - user level
mkdir -p ~/.zsh/completions
dr self completion zsh > ~/.zsh/completions/_dr
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
```

### Outdated completions

After updating the CLI, regenerate completions:

```bash
# Bash
dr self completion bash | sudo tee /etc/bash_completion.d/dr
source ~/.bashrc

# Zsh
dr self completion zsh > ~/.zsh/completions/_dr
rm -f ~/.zcompdump
exec zsh

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish
```

## Completion features

### Intelligent suggestions

Completions are context-aware:

```bash
# Only shows valid subcommands
dr auth <Tab>
# Shows: login logout set-url (not other commands)

# Only shows valid flags
dr run --l<Tab>
# Shows: --list (not all flags)
```

### Description support

In Fish and PowerShell, completions include descriptions:

```fish
$ dr templates <Tab>
list    (List available templates)
setup   (Interactive template setup wizard)
```

### Dynamic completion

Some completions are generated dynamically:

```bash
# Task names from current Taskfile
dr run <Tab>

# Available shells
dr self completion <Tab>
```

## Advanced configuration

### Custom completion scripts

You can extend or modify generated completions:

```bash
# Generate base completion
dr self completion bash > ~/dr-completion-custom.bash

# Edit to add custom logic
vim ~/dr-completion-custom.bash

# Source your custom version
source ~/dr-completion-custom.bash
```

### Completion performance

For faster completions, especially with dynamic suggestions:

```bash
# Cache template list
dr templates list > ~/.dr-templates-cache

# Use cached list in custom completion script
```

## See also

- [Shell completion guide](../user-guide/shell-completions.md)&mdash;detailed setup instructions.
- [Quick start](../../README.md#quick-start)&mdash;initial setup.
- Command completion is powered by [Cobra](https://github.com/spf13/cobra).
