# PancakeStore LE v1.0 — final UI polish

## Changes

- Reduced the gap between the existing Logs footer and the Working Edition attribution from 4 pt to 0 pt so the attribution follows the same natural line spacing as the two lines above it.
- Translated `section.logs.workingEdition` in all 12 bundled localizations. The `ceoca` link target remains `https://t.me/usrlibexec`; the link text stays `ceoca`; the final period is outside the Markdown link so it keeps the normal secondary text color.
- Added the localized `action.downgrading` label in all 12 bundled localizations.
- Kept the Downgrade App button visible while the downgrade operation is in progress. It shows the localized `Downgrading…` label without an icon and is disabled using the existing `FancyButtonStyle()` + `.disabled(...)` mechanism. The existing style uses SwiftUI's `isEnabled` environment value to render disabled foreground and background in gray.
- Deferred the synchronous version lookup to the next main-queue turn so SwiftUI can render the disabled button before the lookup begins.
- After the existing `hasServedApp` success marker is set, the Downgrade App button is replaced by the existing Open App button, preserving the current completion flow.

## Files changed

- `MuffinStoreJailed/ContentView.swift`
- `MuffinStoreJailed/Assets/ar.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/de.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/en.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/es.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/it.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/nb.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/nl.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/ro.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/ru.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/sv.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/vi.lproj/Localizable.strings`
- `MuffinStoreJailed/Assets/zh-Hans.lproj/Localizable.strings`

No changes were made to the bundle identifier, app version, Store authentication, ipatool download/install bridge, or the installation flow.

## Validation

- Swift parser check passed for `ContentView.swift` (Swift 6.2.1 parser mode).
- All 12 localizations contain exactly one `section.logs.workingEdition` and one `action.downgrading` key.
- All 12 attribution values retain the exact `ceoca` link and URL and end in a period after the closing Markdown link.
- Xcode project file was not changed; the existing bundle identifier and marketing version remain as in the supplied LE v1.0 project.
- Full Xcode build and on-device UI test were not available in this environment.
