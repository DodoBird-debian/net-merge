<p align="center">
  <img src="docs/icon.png" width="128" alt="net-merge icon">
</p>

<h1 align="center">net-merge</h1>

<p align="center">
  <a href="https://github.com/DodoBird-debian/net-merge/releases/latest/download/net-merge-setup.exe"><img src="https://img.shields.io/badge/Download-Windows%20Installer-5b38d5?logo=windows&logoColor=white" alt="Download the latest Windows installer"></a>
  <a href="https://github.com/DodoBird-debian/net-merge/releases/latest/download/net-merge-android.apk"><img src="https://img.shields.io/badge/Download-Android%20APK-278c4b?logo=android&logoColor=white" alt="Download the latest Android installer"></a>
</p>

> Rebranded for [Iraq PC Community on Facebook](https://www.facebook.com/groups/1088749137864406/).
> Based on [net-merge by Rohit Mahto](https://github.com/rohit267/net-merge). The original MIT license and copyright notice are preserved.


## Iraq PC Community additions

[الدليل باللغة العربية](README.ar.md)

This fork adds **per-connection data allowances**, **session usage in GB**, and **Arabic/English support**. Existing per-connection speed limits remain available. Downloads for Windows, Linux, macOS, Android and unsigned experimental iOS are on [this fork’s Releases page](https://github.com/DodoBird-debian/net-merge/releases). See the [phone build guide](mobile/README.md) for Android and iOS requirements.

- Open **Connection limits…** from the tray to set a custom speed in Mbps and an allowance in decimal GB for each adapter. Upload and download share one data allowance; the speed cap applies separately to each direction.
- **Used since start** in the menu and Windows hover panel counts only traffic relayed through net-merge. Every tracked connection and the combined total are shown. Session usage survives pause/resume and reconnects, and resets when the process restarts.
- **Allowance used** is saved across restarts and resets only through **Reset used data…**. Changing a limit or selecting Unlimited preserves the recorded usage.
- When an allowance is exhausted, the app closes that adapter's relayed connections and assigns new connections to other eligible adapters. Applications may need to reconnect. If every allowance is exhausted, capture stays active and the app reports the condition instead of reusing a capped link.
- While an allowance is enabled, automatic pause is suspended. A manual pause stops relaying and clearly shows **Paused**. Other traffic then uses your normal network routing and is neither counted nor limited by net-merge.
- Choose **العربية / English** in the tray. Arabic Windows installations default to Arabic. The saved choice also applies to help and messages; the Windows settings and installer use right-to-left layouts.

Useful commands (adapter names or stable IDs are accepted):

```text
net-merge limit "Wi-Fi" 7.5
net-merge quota "Wi-Fi" 10
net-merge quota "Wi-Fi" reset
net-merge quota "Wi-Fi" unlimited
net-merge quota
net-merge status -json
net-merge language ar
net-merge language en
```

`GB` means 1,000,000,000 bytes. Usage combines relayed upload and download payloads, including small UDP packets and IPv4/IPv6. It excludes traffic bound directly to adapters, traffic while paused, protocol overhead, and traffic from other devices: **this is not the ISP's billing total**. Kernel buffers and concurrent I/O can slightly exceed an allowance at cutoff. A crash may lose the latest one-second checkpoint. Unreadable allowance files stop startup; failed saves block relaying until storage recovers. Do not run multiple SOCKS instances against the same state directory.

On macOS, unprivileged SOCKS mode keeps its state in the user's cache under `net-merge-socks`; pass that directory with `-state-dir` to control it. The native limits window and expanded hover panel are Windows features; existing Linux/macOS tray and CLI support remain.

## Downloads

Get the [latest net-merge release for Iraq PC Community](https://github.com/DodoBird-debian/net-merge/releases/latest).
Verify files against the release’s `SHA256SUMS`.

| Platform | Download |
| --- | --- |
| Windows 10/11, x64 | `net-merge-setup-0.4.1.exe` or `net-merge-0.4.1-portable.zip` |
| Linux | CPU-specific tarball, DEB, RPM, Alpine APK or Arch package |
| macOS, Intel and Apple Silicon | `net-merge-0.4.1-macos-universal.tar.gz` |
| Android 8+ | `net-merge-0.4.1-android.apk`, signed with this community fork’s key |
| iOS 16+, arm64 | `net-merge-0.4.1-ios-arm64-unsigned.ipa`, experimental; requires your own signing/provisioning |

Linux CPUs: x86_64, aarch64, armv7, armv6, i686, riscv64, ppc64le, s390x and loongarch64.
Linux `.apk` files are **Alpine packages**, not Android apps.
The Android package uses `org.iraqpc.netmerge`, separately from upstream.
See [phone builds and platform limitations](mobile/README.md).

## Install

On Windows, run the installer, or extract the portable ZIP and launch the tray executable.

On Linux/macOS, install this fork’s release tarball:

```sh
curl -fsSL https://raw.githubusercontent.com/DodoBird-debian/net-merge/main/install.sh | sh
```

The script downloads from **this repository**. It does not add upstream package repositories.
Alternatively install a downloaded Linux package with your distribution’s package manager.
The tray requires StatusNotifierItem/AppIndicator support on Linux.

macOS uses a universal binary with an ad-hoc signature. It is not Apple notarized. The installer
runs `net-merge install` with admin permission to configure system-wide access.

Android requires VPN consent and cannot operate alongside another VPN. The unsigned iOS build
includes the app and packet-tunnel extension; both require signing and appropriate Network
Extension and App Group provisioning before use. The phone ports have not been validated with
two real phone connections; iOS is experimental. CI build and emulator results do not establish
carrier/device support for simultaneous Wi-Fi and cellular.

## Developing

See [desktop development and validation](docs/DEVELOPING.md) and the [phone guide](mobile/README.md).
Build with Go 1.27. `go test ./...` checks the desktop module; phone Go tests live in
`mobile/android/golib`. GitHub Actions builds and stages all platforms before publishing a
complete release. Android releases use the repository’s community signing secrets; the iOS
build deliberately uses `CODE_SIGNING_ALLOWED=NO`.

## Credits and license

Based on [net-merge by Rohit Mahto](https://github.com/rohit267/net-merge) and the
[Android port by SenseiRedsmoke and contributors](https://github.com/net-merge/android).
Rebranded for [Iraq PC Community on Facebook](https://www.facebook.com/groups/1088749137864406/).
The original [MIT license](LICENSE), Android [MIT license](mobile/android/LICENSE), and
[third-party notices](THIRD-PARTY-NOTICES.md) are retained.
