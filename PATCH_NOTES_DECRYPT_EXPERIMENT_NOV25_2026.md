# PancakeStore LE — FairPlay decrypt experiment (Nov 25, 2026)

Base: `PancakeStore-LE-v1.0-first-login-assets-fix-Nov24.zip`.

## What changed

- Added a `Decrypt` action in Settings. It is disabled until PancakeStore has handed off the current downgraded app and has a stored Bundle ID.
- Persisted the last downgraded Bundle ID, version, and handoff state so the Settings action remains available after PancakeStore is relaunched.
- Added an explicit LaunchServices lookup for the selected Bundle ID. The implementation does not enumerate all installed applications.
- Added a Mach-O FairPlay decryption experiment using the iOS `mremap_encrypted` path (including syscall fallback) and `vm_read_overwrite` to collect the remapped pages.
- The installed app bundle is used as the read source so FairPlay can find its original `SC_Info` data. PancakeStore copies the `.app` first and writes decrypted executable bytes only into that copy.
- Attempts the main executable first, then other Mach-O files found in the app bundle (frameworks and extensions). A partial IPA is exported when the main executable succeeds but one or more additional binaries fail.
- Builds `Payload/<App>.app` into an IPA with the existing Zip 2.1.2 dependency and opens the iOS share sheet.

## Important technical distinction

The uploaded `ipatool` source does not decrypt iOS IPA executables through its `StoreAgent`; that path is for macOS package decryption. This Settings experiment therefore does **not** call the ipatool `StoreAgent`. It is an independent implementation of the public `mremap_encrypted` research approach described by FairPlayed.

## Requirements and limitations

- The target app must already be installed on the device and its Apple App Store license must belong to the Apple ID signed into the device.
- Developer Mode must be enabled. The referenced research reports testing on stock iOS 27.0.1.
- The target application's installed `.app` and `SC_Info` must be readable by the current app and the FairPlay service. If LaunchServices cannot resolve the Bundle ID or the system denies access, the action reports an error.
- Decrypted code bytes are written into an exported copy. Existing code signatures are not regenerated, so the output is intended for inspection/research and is not promised to be directly installable.
- This is experimental: success depends on iOS behavior, app packaging, and available FairPlay license data. Do not use it on apps for which you do not hold a valid license, and do not redistribute decrypted apps.

## Validation performed in this environment

- Swift parser check passed for the changed Swift files.
- The C source passed a host Clang syntax check against lightweight mock Apple headers. This is only a syntax check, not an iOS SDK build.
- An Xcode/iOS build and on-device test were not possible in this environment (`xcodebuild` is unavailable).
