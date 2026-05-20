#!/bin/sh
# kungfu installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/mjcurry/kungfu/main/install.sh | sh
#
# Environment variables:
#   KUNGFU_VERSION           Version tag to install (default: latest).
#   KUNGFU_INSTALL_DIR       Install destination (default: /usr/local/bin if
#                            writable, else $HOME/.local/bin).
#   KUNGFU_INSTALL_DEBUG     Set to 1 for verbose output.
#   KUNGFU_INSTALL_BASE_URL  Override the archive base URL. Mainly used by the
#                            install-test CI workflow to install from a local
#                            snapshot. Production users should not set this.
#
# POSIX sh only — tested under dash. No bashisms.

set -eu

REPO="mjcurry/kungfu"

# -- logging -----------------------------------------------------------------

log()   { printf '%s\n' "$*" >&2; }
err()   { printf 'error: %s\n' "$*" >&2; }
debug() {
    if [ -n "${KUNGFU_INSTALL_DEBUG:-}" ]; then
        printf 'debug: %s\n' "$*" >&2
    fi
}

# -- cleanup -----------------------------------------------------------------

TMPDIR_KUNGFU=""
cleanup() {
    if [ -n "$TMPDIR_KUNGFU" ] && [ -d "$TMPDIR_KUNGFU" ]; then
        rm -rf "$TMPDIR_KUNGFU"
    fi
}
trap cleanup EXIT INT TERM HUP

have() { command -v "$1" >/dev/null 2>&1; }

# -- platform detection ------------------------------------------------------

detect_os() {
    case "$(uname -s)" in
        Linux)  echo linux ;;
        Darwin) echo darwin ;;
        *)
            err "unsupported OS: $(uname -s)."
            err "see https://github.com/$REPO/releases for available binaries."
            exit 1
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo amd64 ;;
        aarch64|arm64) echo arm64 ;;
        *)
            err "unsupported architecture: $(uname -m)."
            err "see https://github.com/$REPO/releases for available binaries."
            exit 1
            ;;
    esac
}

# -- downloader --------------------------------------------------------------

DOWNLOADER=""
choose_downloader() {
    if have curl; then
        DOWNLOADER=curl
    elif have wget; then
        DOWNLOADER=wget
    else
        err "neither curl nor wget is available; one of them is required."
        exit 1
    fi
    debug "downloader: $DOWNLOADER"
}

download() {
    # download URL TARGET
    case "$DOWNLOADER" in
        curl) curl -fsSL "$1" -o "$2" ;;
        wget) wget -q -O "$2" "$1" ;;
    esac
}

download_stdout() {
    case "$DOWNLOADER" in
        curl) curl -fsSL "$1" ;;
        wget) wget -qO- "$1" ;;
    esac
}

# -- version discovery -------------------------------------------------------

discover_version() {
    if [ -n "${KUNGFU_VERSION:-}" ]; then
        echo "$KUNGFU_VERSION"
        return
    fi
    api="https://api.github.com/repos/$REPO/releases/latest"
    debug "discovering latest version from $api"
    tag="$(download_stdout "$api" | grep '"tag_name"' | head -1 \
        | sed -e 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//' -e 's/".*//')"
    if [ -z "$tag" ]; then
        err "could not discover latest release from $api"
        exit 1
    fi
    echo "$tag"
}

# -- checksum verification ---------------------------------------------------

verify_checksum() {
    archive="$1"
    checksums="$2"
    name="$(basename "$archive")"
    if have sha256sum; then
        actual="$(sha256sum "$archive" | awk '{print $1}')"
    elif have shasum; then
        actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
    else
        log "warning: neither sha256sum nor shasum available; skipping checksum verification."
        return
    fi
    expected="$(awk -v want="$name" '$2 == want {print $1; exit}' "$checksums")"
    if [ -z "$expected" ]; then
        err "checksum for $name not found in $checksums"
        exit 1
    fi
    if [ "$expected" != "$actual" ]; then
        err "checksum mismatch for $name"
        err "  expected: $expected"
        err "  got:      $actual"
        exit 1
    fi
    debug "checksum verified: $actual"
}

# -- install location --------------------------------------------------------

# Sets INSTALL_DIR and (optionally) PATH_HINT.
INSTALL_DIR=""
PATH_HINT=""
choose_install_dir() {
    if [ -n "${KUNGFU_INSTALL_DIR:-}" ]; then
        mkdir -p "$KUNGFU_INSTALL_DIR" 2>/dev/null || {
            err "cannot create KUNGFU_INSTALL_DIR=$KUNGFU_INSTALL_DIR"
            exit 1
        }
        INSTALL_DIR="$KUNGFU_INSTALL_DIR"
        return
    fi
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
        INSTALL_DIR="/usr/local/bin"
        return
    fi
    fallback="$HOME/.local/bin"
    mkdir -p "$fallback" 2>/dev/null || {
        err "could not create $fallback and /usr/local/bin is not writable."
        err "set KUNGFU_INSTALL_DIR to an explicit destination and re-run."
        exit 1
    }
    INSTALL_DIR="$fallback"
    case ":$PATH:" in
        *":$fallback:"*) ;;
        *) PATH_HINT="$fallback" ;;
    esac
}

# -- main --------------------------------------------------------------------

choose_downloader
os="$(detect_os)"
arch="$(detect_arch)"
version="$(discover_version)"
version_clean="${version#v}"

archive_name="kungfu_${version_clean}_${os}_${arch}.tar.gz"
checksum_name="kungfu_${version_clean}_checksums.txt"
if [ -n "${KUNGFU_INSTALL_BASE_URL:-}" ]; then
    # Test-only override: assume archives live directly under the URL.
    base_url="${KUNGFU_INSTALL_BASE_URL%/}"
else
    base_url="https://github.com/$REPO/releases/download/$version"
fi

TMPDIR_KUNGFU="$(mktemp -d 2>/dev/null || mktemp -d -t kungfu-install)"
debug "tempdir: $TMPDIR_KUNGFU"

log "downloading $archive_name ..."
download "$base_url/$archive_name" "$TMPDIR_KUNGFU/$archive_name"

log "verifying checksum ..."
download "$base_url/$checksum_name" "$TMPDIR_KUNGFU/$checksum_name"
verify_checksum "$TMPDIR_KUNGFU/$archive_name" "$TMPDIR_KUNGFU/$checksum_name"

log "extracting ..."
tar -xzf "$TMPDIR_KUNGFU/$archive_name" -C "$TMPDIR_KUNGFU"

choose_install_dir
target="$INSTALL_DIR/kungfu"

log "installing to $target ..."
mv "$TMPDIR_KUNGFU/kungfu" "$target"
chmod +x "$target"

log "smoke-testing ..."
"$target" version >/dev/null

log ""
log "kungfu $version installed to $target"
if [ -n "$PATH_HINT" ]; then
    log ""
    log "note: $PATH_HINT is not on your PATH. Add this line to your shell rc:"
    log "  export PATH=\"$PATH_HINT:\$PATH\""
fi
log ""
log "run 'kungfu --help' to get started."
