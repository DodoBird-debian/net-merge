#!/bin/sh
# Installs net-merge on macOS or Linux, the easiest way the machine allows:
#
#   curl -fsSL https://raw.githubusercontent.com/DodoBird-debian/net-merge/main/install.sh | sh
#
# Installs the community fork from GitHub release tarballs on Linux and macOS.
# NET_MERGE_NO_START=1 leaves the tray off.
#
# Remove this fork's tarball installation (without touching upstream packages):
#
#   curl -fsSL https://raw.githubusercontent.com/DodoBird-debian/net-merge/main/install.sh | sh -s -- uninstall
set -eu

REPO=https://github.com/DodoBird-debian/net-merge

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf 'net-merge install: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }
fetch() { # fetch URL: the body on stdout
    if have curl; then curl -fsSL "$1"; elif have wget; then wget -qO- "$1"; else die "needs curl or wget"; fi
}

if [ "$(id -u)" = 0 ]; then SUDO=
elif have sudo; then SUDO=sudo
elif have doas; then SUDO=doas
else die "run it as root, or install sudo"; fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# ---- the release tarball, for any CPU net-merge is built for
tarball() {
    if [ "$(uname -s)" = Darwin ]; then
        name=macos-universal
    else
        case $(uname -m) in
            i?86) name=linux-i686 ;;
            armv8l) name=linux-armv7l ;;
            *) name=linux-$(uname -m) ;;
        esac
    fi
    say "Downloading net-merge ($name)"
    fetch "$REPO/releases/latest/download/net-merge-$name.tar.gz" | tar -xz -C "$tmp" net-merge ||
        die "no download for $name; see $REPO/releases"
    if [ "$(uname -s)" = Darwin ]; then
        $SUDO "$tmp/net-merge" install   # copies it to /usr/local/bin, lets the tray start without a password
    else
        $SUDO install -D -m 755 "$tmp/net-merge" /usr/local/bin/net-merge
        if have setcap; then
            $SUDO setcap cap_net_admin,cap_net_raw+ep /usr/local/bin/net-merge
        else
            say "setcap not found: install libcap, then run: sudo setcap cap_net_admin,cap_net_raw+ep /usr/local/bin/net-merge"
        fi
    fi
}

# Earlier community releases did not write an install marker. The fork's release binary embeds its
# own GitHub repository, which lets us recognize those installs without claiming an upstream binary.
community_binary() {
    [ -f "$1" ] && LC_ALL=C grep -aq 'DodoBird-debian/net-merge' "$1"
}

stop_community_tray() {
    if community_binary "$1"; then
        "$1" quit >/dev/null 2>&1 || true
        sleep 2
    fi
}

# ---- uninstall only the community tarball; package managers and their keys belong to their owners
uninstall() {
    case $(uname -s) in
        Darwin)
            bin=/opt/net-merge/bin/net-merge
            community_binary "$bin" || bin=/usr/local/bin/net-merge
            ;;
        Linux) bin=/usr/local/bin/net-merge ;;
        *) die "this script is for macOS and Linux" ;;
    esac
    if ! community_binary "$bin"; then
        say "No Iraq PC Community tarball installation found. Other net-merge installations were left alone."
        return
    fi
    say "Stopping net-merge"
    "$bin" quit >/dev/null 2>&1 || true
    $SUDO "$bin" off >/dev/null 2>&1 || true
    "$bin" autostart off >/dev/null 2>&1 || true
    sleep 2
    if [ "$(uname -s)" = Darwin ]; then
        $SUDO "$bin" uninstall
        $SUDO rm -rf /var/run/net-merge
    else
        $SUDO rm -f "$bin"
    fi
    hash -r 2>/dev/null || true
    say "Community tarball removed. Saved settings remain available for a later install."
}

case ${1:-} in
    uninstall|--uninstall) uninstall; exit 0 ;;
    '') ;;
    *) die "unknown option $1 (use: uninstall)" ;;
esac

# This fork installs its own release tarballs. It does not configure upstream repositories.
case $(uname -s) in
    Darwin)
        if community_binary /opt/net-merge/bin/net-merge; then
            stop_community_tray /opt/net-merge/bin/net-merge
        else
            stop_community_tray /usr/local/bin/net-merge
        fi
        $SUDO mkdir -p /usr/local/bin
        tarball
        ;;
    Linux)
        stop_community_tray /usr/local/bin/net-merge
        tarball
        ;;
    *) die "this script is for macOS and Linux" ;;
esac

have net-merge || PATH=$PATH:/usr/local/bin
say "Installed $(net-merge version)"

# ---- start the tray: on a Mac, or on a Linux desktop session that isn't root's
if [ "${NET_MERGE_NO_START:-}" = 1 ]; then
    say "Start it with: net-merge tray"
elif [ "$(uname -s)" = Darwin ] || { [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] && [ "$(id -u)" != 0 ]; }; then
    nohup net-merge tray >/dev/null 2>&1 &
    say "Started: look for the net-merge icon in the menu bar or tray. To start it at sign-in: net-merge autostart on"
else
    say "Start it with: net-merge tray (from your desktop session), or net-merge run (any terminal, as root)"
fi
