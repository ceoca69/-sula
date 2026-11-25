# PancakeStore native Store-authentication experiment — 23 Nov 2026

## What changed

- Replaced the old `MZFinance.woa/wa/authenticate` sign-in path with the `ipatool` native Store-authentication implementation.
- Embedded the supplied `ipatool` source as an iOS ARM64 Go C-archive, with the upstream iOS Unicorn static-library patch and symbol export needed by `purego`.
- Added a Swift/C bridge that passes credentials and the 2FA code to the Go login routine and receives the account, cookies, keychain state, and stable local machine identity.
- Persisted the `ipatool` Anisette/Store-AMD state and cookies inside PancakeStore's existing SEP-encrypted auth-info storage so the second-factor attempt can continue the same provisioning state.
- Updated the Anisette Client Info and User-Agent to the profile reported in SideStore's Nov 20–21 workaround thread.
- Restored the pending-2FA UI state when the app is relaunched.
- Updated the GitHub Actions build to use macOS 26 / Xcode 26.6 / Go 1.26.5 and build the Go archive before xcodebuild.

## Why SideStore settings are not copied literally

The SideStore Nov 20–22 workaround is for SideStore's AuthKit/ADI sign-in path. It disables on-device Anisette, fetches/saves remote Anisette overrides, selects the `jkcoxson` remote server, and disables server auto-rotation. PancakeStore's new login does not use SideStore's `SignInOperation` or remote SideStore server. Instead, it runs ipatool's Store-native flow, which generates its own Anisette state and Store AMD proof. Only the Anisette client profile values are adapted here; the SideStore remote server settings are not embedded in PancakeStore.

## Important limitations

- This is an experimental login-only integration. The existing download/product lookup code still uses PancakeStore's older download implementation. Successful login does not prove that IPA download, purchase, or version history now work.
- The Go iOS C-archive and the Xcode IPA have not been built end-to-end in this Linux execution environment. The workflow must perform the real macOS/Xcode build.
- A successful compiler run still cannot prove that Apple's current server accepts the iOS Store AMD proof. The next step is an actual login test on a device.
- The iOS machine identity uses a stable app-vendor identifier to derive a locally administered MAC-format value when iOS hides the real interface MAC. Apple's acceptance of this derived value is unverified.
- The first login may download the required pinned Apple runtime/profile assets into the app's cache. Allow network access and sufficient free space. No passwords, 2FA codes, AMD proofs, or cookies should be posted in logs.

## SideStore November settings found in the issue thread

The reported recipe is:

1. SideStore Settings → User Customizations → turn **On-Device Anisette** OFF and restart.
2. Set Client Info to:
   `<MacBookPro13,3> <Mac OS X;27.2;26B118> <com.apple.AuthKit/1 (com.apple.akd/1.0)>`
3. Set User Agent to:
   `akd/1.0 CFNetwork/811.5.4 Darwin/16.7.0`
4. Tap **Save Overrides** five times.
5. Settings → Anisette Servers → select `jkcoxson` (`https://mac-ani.sidestore.io`) and turn **Enable Auto Rotation** OFF.

This is a community workaround, not an official Apple configuration. Reports include users for whom it did not work or who needed a different variant (Remote Server Sync / SideStore New / `Reset adi.pb`).

## Build

Run the included GitHub Actions workflow. It builds:

1. Unicorn 2.1.4 static library for iOS arm64, with the iOS patch from the supplied ipatool source.
2. `ThirdParty/ipatool/bridge/ios` as `GoBridge/libPancakeIPATool.a`.
3. PancakeStore IPA with Xcode 26.6.

If the build fails, share the first actual compiler/linker error and the surrounding 30–50 lines. Do not share credentials or authentication headers.


## Nov 23 hang diagnostics and fix

- Prevented repeated login taps by adding a shared `isAuthenticating` state, a guard in both login actions, a disabled button state, and a progress indicator.
- Serialized calls entering the Go C bridge so accidental concurrent invocations cannot create competing Store runtimes.
- Added stage logs around the C bridge, Go login, Anisette provisioning, Store AMD, Store bag, AppleMediaServices runtime setup, and emulated request execution. Logs do not include credentials, cookies, or proof values.
- Fixed the underlying unbounded-emulator wait: `Backend.Send` previously applied the authentication timeout only to the HTTP transport, after Unicorn had executed the Apple framework. It now derives a deadline context from `request.Timeout` before invoking the emulator, allowing the emulator's existing context checks to interrupt a stalled execution.
- Added a two-minute upper bound for the outer native provisioning/AMD sequence.

This patch is intended to turn an indefinite apparent freeze into a bounded operation with a precise stage/error. A successful Apple login has not been verified.


## Nov 23 — Bag failure propagation / bridge response hardening

- The current test log reached `AppStore.Login` and returned before `Store bag received`. The native Bag endpoint itself was not yet confirmed as successful.
- Added a stage-specific Go error log for Store Bag retrieval, plus safe HTTP status/content-type/redirect metadata and parsed Bag endpoint metadata. The per-install GUID is redacted from transport errors; passwords, 2FA codes, cookies, and proof values are never logged.
- Hardened the Go C-bridge error JSON so even early errors contain non-null `keychain` and `cookies` values and all required response fields.
- Hardened Swift response decoding so omitted/null optional state fields no longer mask the actual Go-side Bag error with a generic missing-key decode error. The bridge reports only JSON byte count and top-level field names.
- Changed Bag parsing to merge nested `urlBag` and root-level fields per key. This handles mixed layouts during Apple's Bag migration instead of letting a partially populated nested dictionary hide root-level SAP settings.
- Added regression cases for `auth.itunes.apple.com/auth/v1/native`, `/native/fast`, query preservation, Apple's root-level Bag layout, and mixed nested/root-level SAP fields.
- The Go login failure is now returned visibly to the app. This patch does not claim the server accepted a login; actual iOS build and Apple endpoint testing remain required.
