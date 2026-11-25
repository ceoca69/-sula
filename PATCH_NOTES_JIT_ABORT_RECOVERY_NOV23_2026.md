> SUPERSEDED: This was an intermediate recovery patch. The later `PATCH_NOTES_TCI_NO_EXEC_PAGES_NOV23_2026.md` replaces the mprotect-based JIT transition with a no-executable-page TCI interpreter build. Do not apply the old Unicorn JIT patch to the TCI build.

# PancakeStore — Unicorn JIT abort recovery (2026-11-23)

## Crash correlation

The supplied LiveContainer crash report records `EXC_CRASH` / `SIGABRT` and the crashing image is the embedded PancakeStore executable. The final login log line is `nativeLogin: preparing Anisette provisioning`, which is the call site immediately before the native Anisette runtime opens Unicorn.

The previously supplied `ThirdParty/ipatool/tools/patches/unicorn-ios.patch` contained an explicit process-wide `abort()` if `mprotect()` failed while switching Unicorn's translation buffer between writable and executable protection. This is the strongest source-level match for the crash. The report has no matching dSYM, so the PancakeStore image offsets cannot be symbolicated conclusively; treat the source correlation as a strong hypothesis, not a proven symbolicated trace.

## Changes

- `ThirdParty/ipatool/tools/patches/unicorn-ios.patch`
  - Removes the `abort()` on JIT translation-buffer `mprotect()` failure.
  - Captures the native `errno` in an exported diagnostic function, `ipatool_unicorn_jit_protection_error()`.
  - Logs the protection failure but lets the call return to the Go layer.
- `ThirdParty/ipatool/internal/sap/unicorn/engine.go`
  - Registers the diagnostic symbol on iOS.
  - Converts a recorded `mprotect` failure into a normal Go error, including the errno and a LiveContainer JIT hint.
  - Adds stage logs around Unicorn API registration, ARM64 backend check, engine creation, CPU model, and lazy engine initialization.
- `ThirdParty/ipatool/internal/sap/unicorn/engine_test.go`
  - Adds a regression test that verifies the native JIT protection errno is returned as a Go error.
- `tools-build-ipatool-ios-archive.sh` and `ThirdParty/ipatool/tools/build-ios.sh`
  - Fail the build if the compiled archive does not export the new diagnostic symbol.

## Important iOS 26+/27 JIT limitation

This change prevents the existing native `abort()` path and makes JIT permission failures diagnosable. It does **not** yet implement the iOS 26+ StikJIT/TXM executable-region allocator protocol (`JIT26PrepareRegion` / `JIT26Detach`). On iOS 26+ with A15+/M2+ hardware, LiveContainer documents that a per-app JIT launch script is required, and StikJIT's integration guide says the app's JIT allocator must cooperate with the script; selecting `universal.js` alone does not retrofit that protocol into an arbitrary JIT engine.

For a first device-side check, configure LiveContainer's JIT provider to StikDebug (or StikDebug (Another LiveContainer) if StikDebug itself runs inside LiveContainer), enable **Launch with JIT** for PancakeStore, and attach a developer-supplied compatible JIT script if available. Do not assume a generic script makes this Unicorn allocator compatible.

If `mprotect` fails, the revised build should now return a message such as `iOS JIT memory protection failed ... errno=...` instead of aborting LiveContainer. That is a crash-recovery/diagnostic improvement, not proof that Unicorn can execute generated code under the current iOS 27 JIT policy. Full iOS 26+/27 allocator integration remains a separate required task if protection is denied.

## Validation limitations

- Shell syntax and Go formatting are checked locally.
- Full iOS/Xcode build and Go tests were not run in this environment. The project requires Go 1.26.5; the available local Go is 1.23.2.
- The pinned Unicorn 2.1.4 source archive could not be fetched in this environment, so application of the unified diff against the upstream archive was not re-run here.
