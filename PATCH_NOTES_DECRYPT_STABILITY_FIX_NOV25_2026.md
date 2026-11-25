# PancakeStore LE — Decrypt stability fixes (November 25, 2026)

## Changes

- Pinned the existing Decrypt control to the bottom of Settings with `safeAreaInset`, so it no longer scrolls away with the Settings list. It remains disabled until a downgraded bundle ID is available and while a decrypt operation is running.
- Corrected ARC ownership annotations for the dynamically invoked `LSApplicationRecord` initializer. The supplied LiveContainer crash report faults in `objc_release_x22`; the faulting register state points at the `LSApplicationRecord` method table, making this initializer call the strongest source-level match for the crash.
- Moved the complete decrypt/copy/ZIP operation from Swift's cooperative task executor to a dedicated utility `DispatchQueue`. UI state changes remain on the main actor.
- Replaced whole-bundle `FileManager.copyItem` with a tree copy that excludes `SC_Info` and `Watch`, preserves symbolic links, and records non-fatal copy failures. The original installed bundle remains the FairPlay source; the copied bundle is the IPA output.
- Fixed the Mach-O candidate sort comparator so it satisfies strict weak ordering.
- Kept the iOS 27 `cryptid=2` + `PROT_READ` FairPlay attempt. No `PROT_EXEC` / `cryptid=1` fallback is used, so this path does not depend on JIT.
- Added an early, readable error when LaunchServices resolves the target into an app data container. The documented FairPlay path cannot read `SC_Info` from app data containers.
- Tightened universal Mach-O magic handling to the byte order used by the FairPlayed implementation.

## Crash report findings

The attached report is a crash of LiveContainer 3.8.10 on iOS 27.0.1. PancakeStore is loaded as an image from `Documents/Applications`. The triggered cooperative worker thread faults with `EXC_BAD_ACCESS / SIGSEGV` in `objc_release_x22`; its register state references the `LSApplicationRecord` method table. The app frame immediately below `objc_release_x22` is PancakeStore image offset 20564. The supplied archive has no matching dSYM, so that frame cannot be symbolicated to a source line; the ARC ownership mismatch in the dynamic init call is a strong match, not a mathematically proven symbolication.

## Validation performed

- `swiftc -frontend -parse` on all project Swift files: passed.
- Standalone Swift type-check of the background queue and bundle-copy helper pattern: passed.
- Objective-C ARC syntax check for `InstalledAppResolver.m` with local API stubs: passed (only warnings from the stubs' qualifier macros).
- C syntax check for `FairPlayMachODecrypt.c` with local Apple API stubs: passed.
- Synthetic host test: plain Mach-O is identified as unencrypted; an encrypted synthetic Mach-O fails safely with `ENOTSUP` on the non-arm64 host.

No Xcode/iOS SDK is available in this environment, so an Xcode build and an on-device FairPlay run were not performed.
