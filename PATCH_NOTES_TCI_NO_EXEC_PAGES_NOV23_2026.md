# PancakeStore — Unicorn TCI no-executable-page build

## Crash evidence

The attached LiveContainer IPS (2026-11-23 10:35:44) reports `EXC_BAD_ACCESS` / `SIGKILL`, termination namespace `CODESIGNING`, indicator `Invalid Page`. The faulting instruction address is `0x1366d4000`; `vmregioninfo` shows the 4 MiB region as `rw-/rw-`, and the kernel says it failed to fault in a page with execute permissions. This is consistent with Unicorn TCG trying to execute a translated host block from a writable, non-executable translation buffer.

This is distinct from the earlier `UC_ERR_ARCH` and TLS failures. Store Bag retrieval has already completed before Anisette runtime starts.

## Fix approach

The iOS build now uses Unicorn's experimental TCI (Tiny Code Interpreter) backend instead of the host AArch64 JIT backend. Guest ARM64 is still enabled with `UNICORN_ARCH=aarch64`, but the host TCG target is changed to `tci`. TCI interprets translated operations in C and does not execute generated machine-code pages. The TCI backport is pinned to these commits from Naville/unicorn:

- `795ba9aa0c02bf22cf432f27fddceadbd30378d3` — adds TCI interpreter source and target.
- `90e62bcb22f4d068cc4185037b83419918c81a15` — exposes `UNICORN_INTERPRETER` in CMake/configure.
- `dc23827e0f649d767467b9bc8bd415ddf89b5eff` — disables executable code-buffer allocation in interpreter mode.
- `53471ef9cf480fab094bf13db3e5d2f9e2c30dc5` — fixes compile-time TCG target state for interpreter mode.

These patches are applied to the SHA-256-verified upstream Unicorn 2.1.4 source archive. The old `unicorn-ios.patch` that changed one mapping RW/RX with `mprotect()` has been removed from the build path; that approach does not satisfy iOS 27 TXM code-signing validation.

## Source files changed

- `ThirdParty/ipatool/tools/prepare-unicorn-tci.sh` — added shared, pinned TCI preparation script.
- `ThirdParty/ipatool/tools/build-ios.sh` — enables and validates TCI.
- `tools-build-ipatool-ios-archive.sh` — enables and validates TCI for PancakeStore's Go C archive.
- `ThirdParty/ipatool/internal/sap/unicorn/engine.go` — removes the obsolete mprotect diagnostic symbol binding and logs TCI mode.
- `ThirdParty/ipatool/internal/sap/unicorn/engine_test.go` — removes the test for the removed JIT shim.
- `ThirdParty/ipatool/tools/patches/unicorn-ios.patch` — removed; it implemented the old RW/RX mprotect toggle and must not be applied in TCI mode.

## Expected behavior

The build log should show `UNICORN_INTERPRETER=ON` and `CONFIG_TCG_INTERPRETER=y`. The static archive must contain the TCI interpreter object. At runtime PancakeStore logs:

`[Unicorn] execution backend=TCI interpreter (host executable JIT disabled)`

If this build is successful, the previous CODESIGNING / Invalid Page failure caused by executing from an RW-only Unicorn JIT page should no longer occur. Anisette behavior and performance still need on-device verification.

## Limitations

TCI is an experimental, non-default Unicorn backend and is slower than native TCG JIT. Its maintainers explicitly describe it as less exercised than JIT backends. This environment cannot run Xcode/iOS compilation or the project's required Go 1.26.5 toolchain, so the full iOS build and runtime have not been tested here. The build scripts deliberately fail if the TCI patchset or interpreter build flags are missing.
