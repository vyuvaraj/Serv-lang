#!/bin/bash
# Serv Installer for macOS/Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/vyuvaraj/Serv-lang/main/release-scripts/install.sh | bash
#
# Options:
#   SERV_VERSION=1.0.0  — specify version (default: latest)
#   SERV_DIR=~/.serv    — install directory (default: ~/.serv)

set -e

REPO="vyuvaraj/Serv-lang"
INSTALL_DIR="${SERV_DIR:-$HOME/.serv}"
VERSION="${SERV_VERSION:-latest}"

echo "Installing Serv..."

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version if needed
if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')
    if [ -z "$VERSION" ]; then
        echo "Failed to fetch latest version. Set SERV_VERSION=x.x.x"
        exit 1
    fi
fi

ARCHIVE="serv-${OS}-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/$ARCHIVE"

echo "  Version: $VERSION"
echo "  Platform: $OS/$ARCH"
echo "  Installing to: $INSTALL_DIR"
echo ""

# Download and extract
echo "  Downloading..."
curl -fsSL "$URL" -o "/tmp/$ARCHIVE"

echo "  Extracting..."
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
tar xzf "/tmp/$ARCHIVE" -C "$INSTALL_DIR"
rm -f "/tmp/$ARCHIVE"

# Make binaries executable
chmod +x "$INSTALL_DIR/serv"
chmod +x "$INSTALL_DIR/serv-lsp"

# Add to PATH
SHELL_RC=""
if [ -f "$HOME/.zshrc" ]; then
    SHELL_RC="$HOME/.zshrc"
elif [ -f "$HOME/.bashrc" ]; then
    SHELL_RC="$HOME/.bashrc"
fi

if [ -n "$SHELL_RC" ]; then
    if ! grep -q "SERV_HOME" "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo "# Serv Programming Language" >> "$SHELL_RC"
        echo "export SERV_HOME=\"$INSTALL_DIR\"" >> "$SHELL_RC"
        echo "export PATH=\"\$SERV_HOME:\$PATH\"" >> "$SHELL_RC"
        echo "  Added to $SHELL_RC"
    fi
fi

echo ""
echo "✓ Serv v$VERSION installed successfully!"
echo ""
echo "  Location:  $INSTALL_DIR"
echo "  SERV_HOME: $INSTALL_DIR"
echo ""
echo "  Restart your terminal (or run: source $SHELL_RC), then:"
echo "    serv init myapp"
echo "    cd myapp"
echo "    serv run main.srv --watch"
echo ""
echo "  Prerequisite: Go 1.18+ must be installed (https://go.dev/dl/)"
