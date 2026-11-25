# PancakeStore LE — Downgrade button visibility fix (November 25, 2026)

## Root cause

`ContentView` rendered the Downgrade button only while `!StoreData.hasServedApp`. That flag is intentionally persisted in `UserDefaults` and becomes `true` when the local installation handoff page is requested. As a result, SwiftUI removed the Downgrade button after a successful handoff and continued hiding it after relaunch, even though the view's local `isDowngrading` state had reset to `false`.

A second state issue could leave the button stuck in its progress state: `downgradeApp()` returns after presenting the version-mode picker, while the local progress flag stayed true on picker cancellation or several lookup/download failures.

## Changes

- Moved the in-progress flag into `StoreData` as a non-persistent `@Published` property so the view and download/install helpers share the same lifecycle state.
- The Downgrade button is now rendered whenever the Store session is logged in. `hasServedApp` no longer controls its visibility.
- Open App is shown independently whenever a successful installation handoff has been recorded; it no longer replaces Downgrade.
- The progress state resets on manual/server picker cancellation, empty/invalid version-history responses, native version lookup failure, local server startup failure, and download/preparation failure.
- The progress state ends when the install handoff endpoint is reached. The success state and Open App continue to use the existing persisted `hasServedApp` marker.
- The Decrypt control was restored to its original Section in the Settings List. No bottom `safeAreaInset` was added to Settings. Its FairPlay implementation and async execution path were retained.

## Validation

- All 8 Swift source files passed `swiftc -frontend -parse`.
- Reviewed the source diff: only `ContentView.swift`, `Downgrader.swift`, and `SettingsView.swift` were changed for this update.
- No Xcode/iOS SDK is available here, so a full iOS build and on-device UI validation were not performed.
