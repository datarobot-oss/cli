#!/bin/sh
# DataRobot CLI uninstallation script for macOS and Linux
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/uninstall.sh | sh
#
#   Or with custom install directory:
#     curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/uninstall.sh | INSTALL_DIR=/custom/path sh

set -e

# Configuration
BINARY_NAME="dr"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Colors for output
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    BOLD=''
    NC=''
fi

# Helper functions
info() {
    printf "${GREEN}==>${NC} ${BOLD}%s${NC}\n" "$1"
}

warn() {
    printf "${YELLOW}Warning:${NC} %s\n" "$1"
}

error() {
    printf "${RED}Error:${NC} %s\n" "$1" >&2
    exit 1
}

step() {
    printf "${BLUE}  →${NC} %s\n" "$1"
}

# Check if binary exists
check_installation() {
    if [ ! -f "$INSTALL_DIR/$BINARY_NAME" ]; then
        error "DataRobot CLI is not installed at $INSTALL_DIR/$BINARY_NAME"
    fi

    if [ -x "$INSTALL_DIR/$BINARY_NAME" ]; then
        INSTALLED_VERSION=$("$INSTALL_DIR/$BINARY_NAME" version 2>/dev/null | head -n1 || echo "unknown version")
        step "Found: $INSTALLED_VERSION"
        step "Location: $INSTALL_DIR/$BINARY_NAME"
    fi
}

# Remove binary
remove_binary() {
    step "Removing binary from $INSTALL_DIR/$BINARY_NAME..."
    if rm -f "$INSTALL_DIR/$BINARY_NAME"; then
        printf "   ${GREEN}✓${NC} Binary removed\n"
    else
        error "Failed to remove binary. Do you have write permissions to $INSTALL_DIR?"
    fi
}

# Remove PATH entries from shell profiles
remove_from_path() {
    local modified=0

    # List of common shell profile files
    local profiles="$HOME/.zshrc $HOME/.bashrc $HOME/.bash_profile $HOME/.profile $HOME/.config/fish/config.fish"

    for profile in $profiles; do
        if [ -f "$profile" ]; then
            # Check if file contains reference to INSTALL_DIR
            if grep -q "$INSTALL_DIR" "$profile" 2>/dev/null; then
                step "Found PATH reference in $profile"

                # Create backup
                cp "$profile" "$profile.bak.$(date +%Y%m%d_%H%M%S)"

                # Remove lines containing INSTALL_DIR and the comment line before it if it's the DataRobot installer comment
                sed -i.tmp '/# Added by DataRobot CLI installer/d' "$profile" 2>/dev/null || sed -i '' '/# Added by DataRobot CLI installer/d' "$profile" 2>/dev/null || true
                sed -i.tmp "\|$INSTALL_DIR|d" "$profile" 2>/dev/null || sed -i '' "\|$INSTALL_DIR|d" "$profile" 2>/dev/null || true
                rm -f "$profile.tmp"

                printf "   ${GREEN}✓${NC} Removed from $profile\n"
                modified=1
            fi
        fi
    done

    if [ $modified -eq 1 ]; then
        echo ""
        warn "Shell profiles were modified. Restart your shell or run:"
        case "$SHELL" in
            */zsh)
                echo "  ${BLUE}source ~/.zshrc${NC}"
                ;;
            */bash)
                echo "  ${BLUE}source ~/.bashrc${NC}"
                ;;
            *)
                echo "  ${BLUE}source your shell profile${NC}"
                ;;
        esac
    else
        step "No PATH entries found in shell profiles"
    fi
}

# Remove shell completions
remove_completions() {
    local removed=0

    # Zsh completions
    local zsh_locations="
        $HOME/.oh-my-zsh/custom/completions/_$BINARY_NAME
        $HOME/.zsh/completions/_$BINARY_NAME
    "

    for location in $zsh_locations; do
        if [ -f "$location" ]; then
            rm -f "$location"
            step "Removed Zsh completion from $location"
            removed=1
        fi
    done

    # Clear Zsh completion cache
    if [ $removed -eq 1 ]; then
        rm -f "$HOME/.zcompdump"* 2>/dev/null
    fi

    # Bash completions
    local bash_locations="
        $HOME/.bash_completions/$BINARY_NAME
        /etc/bash_completion.d/$BINARY_NAME
    "

    for location in $bash_locations; do
        if [ -f "$location" ]; then
            if [ -w "$location" ]; then
                rm -f "$location"
                step "Removed Bash completion from $location"
                removed=1
            else
                step "Skipping $location (no write permission)"
            fi
        fi
    done

    # Fish completions
    local fish_completion="$HOME/.config/fish/completions/$BINARY_NAME.fish"
    if [ -f "$fish_completion" ]; then
        rm -f "$fish_completion"
        step "Removed Fish completion from $fish_completion"
        removed=1
    fi

    if [ $removed -eq 0 ]; then
        step "No shell completions found"
    fi
}

# Confirm uninstallation
confirm_uninstall() {
    if [ -t 0 ]; then  # Check if stdin is a terminal (interactive)
        echo ""
        printf "${YELLOW}${BOLD}Are you sure you want to uninstall DataRobot CLI? [y/N]${NC} "
        read -r response
        case "$response" in
            [yY][eE][sS]|[yY])
                return 0
                ;;
            *)
                info "Uninstallation cancelled"
                exit 0
                ;;
        esac
    fi
    # Non-interactive mode, proceed
    return 0
}

# Main uninstallation flow
main() {
    cat << "EOF"
    ____        __        ____        __          __
   / __ \____ _/ /_____ _/ __ \____  / /_  ____  / /_
  / / / / __ `/ __/ __ `/ /_/ / __ \/ __ \/ __ \/ __/
 / /_/ / /_/ / /_/ /_/ / _, _/ /_/ / /_/ / /_/ / /_
/_____/\__,_/\__/\__,_/_/ |_|\____/_.___/\____/\__/

EOF

    info "Uninstalling DataRobot CLI"
    echo ""

    check_installation
    echo ""

    confirm_uninstall
    echo ""

    info "Removing DataRobot CLI..."
    remove_binary

    echo ""
    info "Checking shell profiles..."
    remove_from_path

    echo ""
    info "Removing shell completions..."
    remove_completions

    echo ""
    info "Uninstallation complete!"
    step "DataRobot CLI has been removed from your system"
    echo ""
}

main "$@"
