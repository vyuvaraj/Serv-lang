#!/bin/bash
# Cross-compile Serv for all target platforms with full runtime bundle.
# Usage: ./release-scripts/build-release.sh v1.0.0
#
# Prerequisites: Go 1.18+ installed
# Outputs: release/ directory with platform-specific archives
#
# Each archive contains:
#   serv (or serv.exe)       — the compiler
#   serv-lsp (or serv-lsp.exe) — language server
#   runtime/                 — Go runtime source (needed for compilation)
#   stdlib/                  — Serv standard library modules
#   declarations/            — Go package declarations
#   go.mod, go.sum           — Go module files (needed for compilation)

set -e

VERSION=${1:-"dev"}
OUTDIR="release"

echo "Building Serv $VERSION for all platforms..."
echo ""
rm -rf "$OUTDIR"
mkdir -p "$OUTDIR"

# Platforms: os/arch
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS="${PLATFORM%/*}"
    GOARCH="${PLATFORM#*/}"
    
    EXT=""
    if [ "$GOOS" = "windows" ]; then
        EXT=".exe"
    fi

    ARCHIVE_NAME="serv-${GOOS}-${GOARCH}"
    STAGE_DIR="$OUTDIR/staging-${GOOS}-${GOARCH}"

    echo "  Building $GOOS/$GOARCH..."
    
    # Create staging directory with full bundle
    mkdir -p "$STAGE_DIR"

    # Build compiler
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w -X main.version=$VERSION" -o "$STAGE_DIR/serv${EXT}" main.go
    
    # Build LSP
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$STAGE_DIR/serv-lsp${EXT}" ./lsp/

    # Copy runtime source (needed for go build to work)
    cp -r runtime "$STAGE_DIR/runtime"

    # Copy stdlib
    cp -r stdlib "$STAGE_DIR/stdlib"

    # Copy declarations
    if [ -d "declarations" ]; then
        cp -r declarations "$STAGE_DIR/declarations"
    fi

    # Copy go.mod and go.sum (needed for module resolution)
    cp go.mod "$STAGE_DIR/go.mod"
    cp go.sum "$STAGE_DIR/go.sum"

    # Package
    if [ "$GOOS" = "windows" ]; then
        (cd "$OUTDIR" && zip -r "${ARCHIVE_NAME}.zip" "staging-${GOOS}-${GOARCH}/" && mv "${ARCHIVE_NAME}.zip" "${ARCHIVE_NAME}.zip")
        # Re-package with a clean top-level directory name
        rm -f "$OUTDIR/${ARCHIVE_NAME}.zip"
        (cd "$STAGE_DIR" && zip -r "../${ARCHIVE_NAME}.zip" .)
    else
        tar czf "$OUTDIR/${ARCHIVE_NAME}.tar.gz" -C "$STAGE_DIR" .
    fi

    # Cleanup staging
    rm -rf "$STAGE_DIR"
done

echo ""
echo "Release archives:"
ls -lh "$OUTDIR"/*.{tar.gz,zip} 2>/dev/null
echo ""

# Print SHA256 hashes for Homebrew/Scoop
echo "SHA256 hashes (for Homebrew formula and Scoop manifest):"
echo "------"
for f in "$OUTDIR"/*.tar.gz "$OUTDIR"/*.zip; do
    if [ -f "$f" ]; then
        echo "  $(basename $f): $(shasum -a 256 "$f" | awk '{print $1}')"
    fi
done
echo ""

echo "Next steps:"
echo "  1. Create GitHub release with tag $VERSION"
echo "  2. Upload archives from release/"
echo "  3. Copy SHA256 hashes into release-scripts/homebrew/serv.rb"
echo "  4. Copy SHA256 hash into release-scripts/scoop/serv.json"
echo "  5. Push Homebrew tap + Scoop bucket repos"
echo ""
echo "Install instructions for users:"
echo "  macOS:   brew tap user/serv && brew install serv"
echo "  Windows: scoop bucket add serv <url> && scoop install serv"
echo "  Linux:   tar xzf serv-linux-amd64.tar.gz -C ~/.serv && export PATH=~/.serv:\$PATH"
