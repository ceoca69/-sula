# Unicorn ARM64 guest backend fix — 2026-11-23

## Root cause

The iOS build scripts compiled the Unicorn host library for iOS arm64 (`CMAKE_OSX_ARCHITECTURES=arm64`) but explicitly selected `-DUNICORN_ARCH=x86`. These are separate settings: the first selects the CPU architecture of the iOS library, while the second selects which guest CPU emulators Unicorn includes.

The Anisette runtime opens `UC_ARCH_ARM64` (numeric architecture ID 2), but the linked Unicorn library had only the x86 guest backend enabled. Unicorn therefore returned `UC_ERR_ARCH` from `uc_open`.

## Changes

- `tools-build-ipatool-ios-archive.sh`: selects `UNICORN_ARCH=aarch64`, explicitly enables the legacy all-in-one static archive, emits compile commands, and checks that `UNICORN_HAS_ARM64` and the AArch64 translator object are present.
- `ThirdParty/ipatool/tools/build-ios.sh`: applies the same AArch64 guest-backend and all-in-one archive configuration.
- `ThirdParty/ipatool/internal/sap/unicorn/engine.go`: checks `uc_arch_supported(UC_ARCH_ARM64)` before calling `uc_open`, and reports a direct build-configuration error if the backend is absent.

## Expected result

On a correctly rebuilt IPA, Unicorn should pass engine creation and proceed to CPU configuration / Anisette runtime setup. This does not guarantee that the later Anisette or Apple native-authentication steps will succeed; those are subsequent stages.
