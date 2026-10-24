#!/bin/sh
# Installs net-merge on macOS or Linux, the easiest way the machine allows:
#
#   curl -fsSL https://raw.githubusercontent.com/DodoBird-debian/net-merge/main/install.sh | sh
#
# Installs the community fork from GitHub release tarballs on Linux and macOS.
# NET_MERGE_NO_START=1 leaves the tray off.
#
# Uninstall, however it was installed:
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

# ---- uninstall: stop it, then remove it the way it was installed, with the repository it came from
uninstall() {
    say "Stopping net-merge"
    if have net-merge; then
        net-merge quit >/dev/null 2>&1 || true           # the tray
        $SUDO net-merge off >/dev/null 2>&1 || true      # system-wide mode started from a terminal
        net-merge autostart off >/dev/null 2>&1 || true  # start at sign-in
        sleep 2
    fi
    if [ "$(uname -s)" = Darwin ]; then
        for exe in /opt/net-merge/bin/net-merge /usr/local/bin/net-merge; do
            [ -x "$exe" ] && { $SUDO "$exe" uninstall || true; break; }   # the root-owned copy and the sudoers rule
        done
        if have brew && brew list --formula net-merge >/dev/null 2>&1; then
            say "Removing with Homebrew"
            brew uninstall net-merge
            brew untap rohit267/tap >/dev/null 2>&1 || true
        fi
        $SUDO rm -rf /var/run/net-merge "/Library/Application Support/net-merge" "$HOME/Library/Logs/net-merge.log"
    else
        if have dpkg && dpkg -s net-merge >/dev/null 2>&1; then
            say "Removing with apt"
            $SUDO env DEBIAN_FRONTEND=noninteractive apt-get remove -y -qq net-merge
            $SUDO rm -f /etc/apt/sources.list.d/net-merge.list /usr/share/keyrings/net-merge.gpg
        elif have rpm && rpm -q net-merge >/dev/null 2>&1; then
            if have zypper; then
                say "Removing with zypper"
                $SUDO zypper --non-interactive remove net-merge
                $SUDO zypper --non-interactive removerepo net-merge >/dev/null 2>&1 || true
            else
                pm=dnf; have dnf || pm=yum
                say "Removing with $pm"
                $SUDO "$pm" remove -y net-merge
                $SUDO rm -f /etc/yum.repos.d/net-merge.repo
            fi
        elif have apk && apk info -e net-merge >/dev/null 2>&1; then
            say "Removing with apk"
            $SUDO apk del net-merge
            $SUDO sed -i "\#^$BASE/apk\$#d" /etc/apk/repositories
            $SUDO rm -f /etc/apk/keys/net-merge.rsa.pub
        elif have pacman && pacman -Q net-merge >/dev/null 2>&1; then
            say "Removing with pacman"
            $SUDO pacman -R --noconfirm net-merge
            $SUDO sed -i '/^\[net-merge\]$/,/^Server = .*net-merge/d' /etc/pacman.conf
            $SUDO pacman-key --delete "$FPR" >/dev/null 2>&1 || true
        fi
        if have brew && brew list --formula net-merge >/dev/null 2>&1; then
            say "Removing with Homebrew"
            brew uninstall net-merge
            brew untap rohit267/tap >/dev/null 2>&1 || true
        fi
        $SUDO rm -f /usr/local/bin/net-merge   # the tarball
        rm -rf "${XDG_CACHE_HOME:-$HOME/.cache}/net-merge"
    fi
    hash -r 2>/dev/null || true
    have net-merge && die "a net-merge is still at $(command -v net-merge); remove it by hand"
    say "net-merge is uninstalled. Your network settings are back to normal."
}

case ${1:-} in
    uninstall|--uninstall) uninstall; exit 0 ;;
    '') ;;
    *) die "unknown option $1 (use: uninstall)" ;;
esac

# This fork installs its own release tarballs. It does not configure upstream repositories.
case $(uname -s) in
    Darwin) net-merge quit 2>/dev/null || true; $SUDO mkdir -p /usr/local/bin; tarball ;;
    Linux) tarball ;;
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
