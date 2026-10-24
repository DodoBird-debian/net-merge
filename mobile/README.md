# Community phone ports

Rebranded for [Iraq PC Community on Facebook](https://www.facebook.com/groups/1088749137864406/).

Android is derived from [net-merge/android](https://github.com/net-merge/android), snapshot
`cc8412f6f7b6b80c06f5492d3d32aad3dff8fc5e`. Its original MIT license is retained in
[android/LICENSE](android/LICENSE). Desktop engine, state and message catalogs are copied
from this repository, with only module import paths changed. Mobile adapter and lifecycle
code live beside them. Keep accounting fixes in both modules synchronized.

Android requires Android 8 or later. Install `net-merge-0.4.0-android.apk`; its community
application ID and signing certificate are separate from upstream. Accept Android's VPN
consent prompt. Other VPNs cannot run at the same time. Only Wi-Fi and cellular networks
reported as available by Android can carry flows. Interface identity persists across network
handle changes; changing SIMs on the same modem does not automatically reset the allowance.

The new iOS port is an **experimental unsigned build**, for iOS 16 or later. The IPA contains
an app and a real `NEPacketTunnelProvider` extension. The extension bridges packets through
the same Go TCP/UDP relay, binds sockets to physical `en0` / `pdp_ip*` interfaces, and never
selects a `utun` interface. The app communicates limits, resets, pause and status to the
extension. An unsigned IPA cannot be installed and run directly: sign **both** app and
extension with your own identities/provisioning, matching packet-tunnel and App Group
entitlements. Change the bundle IDs and shared App Group consistently if provisioning
requires it. No Apple Developer account or provisioning credentials are included.

Build Android with Java 17, Android SDK 36/NDK, Go 1.27 and `android/scripts/build-go.sh`,
then `android/gradlew assembleRelease`. Release CI requires the repository's community
Android keystore secrets. Keep a secure backup of the signing key to retain update compatibility.
Build iOS on macOS with Xcode, XcodeGen and Go: run `ios/build.sh`. This intentionally sets
`CODE_SIGNING_ALLOWED=NO` and produces an unsigned device IPA.

Both interfaces have English/Arabic selection, RTL layout, Arabic-digit numeric input,
separate upload/download speed caps, combined relayed-data allowances, manual reset with
confirmation, GB-only session totals and saved allowance usage. Allowances are not the
ISP billing total. Buffered data and the last unsaved one-second checkpoint affect cutoff.
On Android, Pause stops the VPN; device traffic then follows normal OS routing. On iOS,
Pause retains the tunnel and blocks relaying until Resume. Session counters reset when the
engine process restarts; allowance usage persists. Allowances are per physical interface,
not per Wi-Fi SSID or provider contract.

CI verifies Go accounting/concurrency, compilation, catalog completeness, Android lint,
APK signing and unsigned IPA structure. Real dual-network phone validation is still required;
an iOS compile is not evidence that a particular carrier/device permits simultaneous Wi-Fi
and cellular. No physical iPhone or Android phone was available for this release.
