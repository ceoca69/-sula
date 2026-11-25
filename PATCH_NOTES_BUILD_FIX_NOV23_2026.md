# PancakeStore native-auth build fix — 23 Nov 2026

## Root cause in the supplied Actions log

Swift compilation completed. The final link command contained `-lPancakeIPATool`, but no `-L.../GoBridge` search path. The target had a file reference to `GoBridge/libPancakeIPATool.a`, yet Xcode translated it into a library-name lookup that could not locate the archive. The build stopped with `ld: library 'PancakeIPATool' not found`.

## Changes

- Added `$(PROJECT_DIR)/GoBridge` to `LIBRARY_SEARCH_PATHS` for both Debug and Release app target configurations.
- Added preflight checks to `ipabuild.sh` for the Go bridge archive, generated C header, and Unicorn archive. Missing outputs now produce an actionable error before invoking Xcode.
- Added a workflow verification step that checks the three generated artifacts and prints archive metadata before the Xcode build.

## Non-blocking diagnostics in the supplied log

- The AppIntents metadata message is informational; the app does not link AppIntents.framework.
- Deprecation warnings in `Downgrader.swift` and `IPATool.swift` did not stop compilation and are not the cause of this failure.
- No Swift compiler errors were reported in the supplied log.

## Validation status

The project and scripts were statically inspected and shell syntax checked. This environment does not contain Xcode, so an end-to-end iOS link and IPA build must still be run by GitHub Actions.
