#!/bin/sh
# DataRobot CLI installation script for macOS and Linux
#
# Usage:
#   Install latest version:
#     curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | sh
#
#   Install specific version:
#     curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | sh -s -- v0.1.0
#
#   Custom install directory:
#     curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | INSTALL_DIR=/custom/path sh

set -e

# Configuration
REPO="datarobot-oss/cli"
BINARY_NAME="dr"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${1:-latest}"

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

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux*)
            OS="Linux"
            ;;
        darwin*)
            OS="Darwin"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="x86_64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        riscv64)
            ARCH="riscv64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    step "Detected platform: $OS $ARCH"
}

# Check for required tools
check_requirements() {
    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        error "Neither curl nor wget is available. Please install one of them."
    fi

    if ! command -v tar >/dev/null 2>&1; then
        error "tar is not available. Please install it."
    fi
}

# Get the latest release version or validate specified version
get_version() {
    if [ "$VERSION" = "latest" ]; then
        step "Fetching latest version..."
        if command -v curl >/dev/null 2>&1; then
            VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        else
            VERSION=$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        fi

        if [ -z "$VERSION" ]; then
            error "Failed to fetch the latest version from GitHub"
        fi
    else
        step "Using specified version: $VERSION"
    fi

    # Ensure version starts with 'v'
    case "$VERSION" in
        v*) ;;
        *) VERSION="v$VERSION" ;;
    esac

    printf "   ${BOLD}Version:${NC} %s\n" "$VERSION"
}

# Compare versions (returns 0 if v1 < v2, 1 if v1 >= v2)
compare_versions() {
    local v1=$1
    local v2=$2

    # Remove 'v' prefix and extract version numbers
    v1=$(echo "$v1" | sed 's/^v//')
    v2=$(echo "$v2" | sed 's/^v//')

    # Compare versions
    if [ "$v1" = "$v2" ]; then
        return 1  # Same version
    fi

    # Use sort -V for version comparison if available
    if printf '%s\n' "$v1" "$v2" | sort -V -C 2>/dev/null; then
        return 0  # v1 < v2 (update available)
    else
        return 1  # v1 >= v2 (no update needed)
    fi
}

# Check if binary is already installed
check_existing_installation() {
    if [ -x "$INSTALL_DIR/$BINARY_NAME" ]; then
        CURRENT_VERSION=$("$INSTALL_DIR/$BINARY_NAME" version 2>/dev/null | head -n1 || echo "unknown")

        # Extract just the version number (e.g., "v1.2.3" from "dr version v1.2.3")
        CURRENT_VERSION=$(echo "$CURRENT_VERSION" | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+' | head -n1)

        if [ -z "$CURRENT_VERSION" ] || [ "$CURRENT_VERSION" = "unknown" ]; then
            warn "Unable to determine current version"
            step "Proceeding with installation of $VERSION"
            return 0
        fi

        # Normalize versions (ensure both have 'v' prefix)
        case "$CURRENT_VERSION" in
            v*) ;;
            *) CURRENT_VERSION="v$CURRENT_VERSION" ;;
        esac

        step "Current installation: $CURRENT_VERSION"
        step "Target version: $VERSION"

        # Check if versions are the same
        if [ "$CURRENT_VERSION" = "$VERSION" ]; then
            info "DataRobot CLI $VERSION is already installed"
            step "Installation location: $INSTALL_DIR/$BINARY_NAME"

            if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
                warn "$INSTALL_DIR is not in your PATH"
                show_path_instructions
            fi

            echo ""
            info "Already up to date!"
            exit 0
        fi

        # Check if update is available
        if compare_versions "$CURRENT_VERSION" "$VERSION"; then
            info "Update available: $CURRENT_VERSION → $VERSION"

            # Ask user if they want to update (only in interactive mode)
            if [ -t 0 ]; then
                echo ""
                printf "${BOLD}Would you like to upgrade to $VERSION? [Y/n]${NC} "
                read -r response
                case "$response" in
                    [nN][oO]|[nN])
                        info "Installation cancelled"
                        exit 0
                        ;;
                    *)
                        echo ""
                        info "Upgrading DataRobot CLI..."
                        ;;
                esac
            else
                step "Upgrading to: $VERSION"
            fi
        else
            warn "Target version ($VERSION) is older than current version ($CURRENT_VERSION)"

            # Ask user if they want to downgrade
            if [ -t 0 ]; then
                echo ""
                printf "${BOLD}Would you like to downgrade to $VERSION? [y/N]${NC} "
                read -r response
                case "$response" in
                    [yY][eE][sS]|[yY])
                        echo ""
                        info "Downgrading DataRobot CLI..."
                        ;;
                    *)
                        info "Installation cancelled"
                        exit 0
                        ;;
                esac
            else
                step "Downgrading to: $VERSION"
            fi
        fi
    fi
}

# Download and extract the binary
download_and_install() {
    # Construct download URL
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE_NAME"

    step "Downloading from GitHub..."
    printf "   ${DOWNLOAD_URL}\n"

    # Create temporary directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

    # Download archive
    if command -v curl >/dev/null 2>&1; then
        if ! curl --fail --location --progress-bar "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE_NAME"; then
            error "Failed to download binary. Please check the version exists: https://github.com/$REPO/releases"
        fi
    else
        if ! wget --show-progress -q "$DOWNLOAD_URL" -O "$TMP_DIR/$ARCHIVE_NAME"; then
            error "Failed to download binary. Please check the version exists: https://github.com/$REPO/releases"
        fi
    fi

    # Extract archive
    step "Extracting binary..."
    if ! tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"; then
        error "Failed to extract archive"
    fi

    # Create install directory if it doesn't exist
    if [ ! -d "$INSTALL_DIR" ]; then
        step "Creating install directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR" || error "Failed to create install directory: $INSTALL_DIR"
    fi

    # Install binary
    step "Installing binary to $INSTALL_DIR/$BINARY_NAME..."
    if ! mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"; then
        error "Failed to install binary. Do you have write permissions to $INSTALL_DIR?"
    fi

    if ! chmod +x "$INSTALL_DIR/$BINARY_NAME"; then
        error "Failed to make binary executable"
    fi
}

# Show PATH configuration instructions
# Add directory to PATH in shell profile
add_to_path() {
    local shell_profile=""
    local path_cmd=""

    # Detect shell and set appropriate profile file
    case "$SHELL" in
        */zsh)
            shell_profile="$HOME/.zshrc"
            path_cmd="export PATH=\"\$PATH:$INSTALL_DIR\""
            ;;
        */bash)
            shell_profile="$HOME/.bashrc"
            path_cmd="export PATH=\"\$PATH:$INSTALL_DIR\""
            ;;
        */fish)
            shell_profile="$HOME/.config/fish/config.fish"
            path_cmd="fish_add_path $INSTALL_DIR"
            ;;
        *)
            shell_profile="$HOME/.profile"
            path_cmd="export PATH=\"\$PATH:$INSTALL_DIR\""
            ;;
    esac

    # Add to PATH
    if [ -n "$shell_profile" ]; then
        # Check if already in profile
        if [ -f "$shell_profile" ] && grep -q "$INSTALL_DIR" "$shell_profile" 2>/dev/null; then
            step "PATH entry already exists in $shell_profile"
            return 0
        fi

        # Add to profile
        echo "" >> "$shell_profile"
        echo "# Added by DataRobot CLI installer" >> "$shell_profile"
        echo "$path_cmd" >> "$shell_profile"

        info "Added $INSTALL_DIR to PATH in $shell_profile"
        step "Restart your shell or run: ${BOLD}source $shell_profile${NC}"
        return 0
    else
        warn "Could not detect shell profile"
        return 1
    fi
}

show_path_instructions() {
    echo ""
    echo "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo "${BOLD}Next step: Add to PATH${NC}"
    echo ""
    echo "To use ${BOLD}$BINARY_NAME${NC} from anywhere, add this to your shell profile:"
    echo ""

    # Detect shell and provide specific instructions
    case "$SHELL" in
        */zsh)
            echo "  ${BLUE}echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.zshrc${NC}"
            echo "  ${BLUE}source ~/.zshrc${NC}"
            ;;
        */bash)
            echo "  ${BLUE}echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.bashrc${NC}"
            echo "  ${BLUE}source ~/.bashrc${NC}"
            ;;
        */fish)
            echo "  ${BLUE}fish_add_path $INSTALL_DIR${NC}"
            ;;
        *)
            echo "  ${BLUE}export PATH=\"\$PATH:$INSTALL_DIR\"${NC}"
            ;;
    esac

    echo ""
    echo "Or use the full path: ${BLUE}$INSTALL_DIR/$BINARY_NAME${NC}"
    echo "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Verify installation
verify_installation() {
    if [ -x "$INSTALL_DIR/$BINARY_NAME" ]; then
        step "Verifying installation..."
        INSTALLED_VERSION=$("$INSTALL_DIR/$BINARY_NAME" version 2>/dev/null | head -n1 || echo "installed")
        printf "   ${GREEN}✓${NC} %s\n" "$INSTALLED_VERSION"
    else
        error "Binary not found at $INSTALL_DIR/$BINARY_NAME"
    fi
}

# Prompt user to install shell completions
prompt_completion_install() {
    local shell_name=""

    # Detect shell name for display
    case "$SHELL" in
        */zsh) shell_name="Zsh" ;;
        */bash) shell_name="Bash" ;;
        */fish) shell_name="Fish" ;;
        *) shell_name="your shell" ;;
    esac

    # Only prompt in interactive mode
    if [ -t 0 ]; then
        echo ""
        printf "${BOLD}Would you like to install shell completions for $shell_name? [Y/n]${NC} "
        read -r response

        case "$response" in
            [nN][oO]|[nN])
                echo ""
                info "Skipping completion installation"
                step "You can install completions later with:"
                printf "   ${BLUE}$BINARY_NAME completion install --yes${NC}\n"
                return 1
                ;;
            *)
                echo ""
                info "Installing shell completions..."
                return 0
                ;;
        esac
    else
        # Non-interactive mode, skip completions
        step "To install completions, run: $BINARY_NAME completion install --yes"
        return 1
    fi
}

# Install shell completions using the built-in command
install_completions() {
    # Update PATH temporarily for this script
    export PATH="$INSTALL_DIR:$PATH"

    # Use the interactive completion installer
    if "$INSTALL_DIR/$BINARY_NAME" completion install --yes 2>/dev/null; then
        printf "   ${GREEN}✓${NC} Shell completions installed successfully\n"
        step "Restart your shell to activate completions"
        return 0
    else
        warn "Failed to install completions automatically"
        step "Install manually with: $BINARY_NAME completion install --yes"
        return 1
    fi
}

# Check if install directory is in PATH
check_path() {
    if echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
        step "Installation directory is in PATH"
        return 0
    else
        warn "$INSTALL_DIR is not in your PATH"

        # Ask user if they want to add to PATH automatically
        if [ -t 0 ]; then  # Check if stdin is a terminal (interactive)
            echo ""
            printf "${BOLD}Would you like to add $INSTALL_DIR to your PATH automatically? [y/N]${NC} "
            read -r response
            case "$response" in
                [yY][eE][sS]|[yY])
                    echo ""
                    if add_to_path; then
                        return 0
                    else
                        show_path_instructions
                        return 1
                    fi
                    ;;
                *)
                    echo ""
                    show_path_instructions
                    return 1
                    ;;
            esac
        else
            # Non-interactive mode, just show instructions
            show_path_instructions
            return 1
        fi
    fi
}

# Main installation flow
main() {
      cat << "EOF"
    ____        __        ____        __          __
   / __ \____ _/ /_____ _/ __ \____  / /_  ____  / /_
  / / / / __ `/ __/ __ `/ /_/ / __ \/ __ \/ __ \/ __/
 / /_/ / /_/ / /_/ /_/ / _, _/ /_/ / /_/ / /_/ / /_
/_____/\__,_/\__/\__,_/_/ |_|\____/_.___/\____/\__/

EOF
    info "Installing DataRobot CLI"
    echo ""

    check_requirements
    detect_platform
    get_version
    check_existing_installation

    echo ""
    info "Downloading and installing..."
    download_and_install

    echo ""
    info "Installation complete!"
    verify_installation

    IN_PATH=0
    check_path && IN_PATH=1

    # Prompt for completion installation
    if prompt_completion_install; then
        install_completions
    fi

    echo ""
    if [ $IN_PATH -eq 1 ]; then
        printf "${GREEN}==>${NC} ${BOLD}Get started by running:${NC} ${BOLD}$BINARY_NAME --help${NC}\n"
    else
        printf "${GREEN}==>${NC} ${BOLD}Get started by running:${NC} ${BOLD}$INSTALL_DIR/$BINARY_NAME --help${NC}\n"
    fi
    echo ""
}

main "$@"
