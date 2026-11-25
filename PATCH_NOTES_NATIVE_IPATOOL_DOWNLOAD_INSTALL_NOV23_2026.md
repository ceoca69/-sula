# Native ipatool download and iOS installation handoff

## Summary

This patch connects PancakeStore's existing native ipatool Store session to the actual App Store package download, then hands the prepared IPA to the existing iOS `itms-services` installation flow.

The native authentication and version-history paths are retained. The old Swift `StoreClient.download()` path is no longer used by the selected-version download action.

## Download pipeline

1. Swift restores `nativeKeychain`, `nativeCookies`, and the saved machine identity from the existing SEP-encrypted `authinfo` state.
2. `PancakeIPAToolDownload` passes the selected App Store app ID and `externalVersionID` into the Go bridge.
3. Go restores the ipatool account, calls `appstore.Download()` for `PlatformIPhone`, and refreshes the native Store login once if Apple reports an expired password token.
4. If Apple reports that a license is missing, the bridge checks the App Store catalog. It only attempts to acquire a free-app license when the lookup returns the same app ID and a zero price. It refuses to automatically purchase paid or unverified apps.
5. ipatool adds `iTunesMetadata.plist`; `ReplicateSinf()` injects the FairPlay `sinf` data into the IPA.
6. The bridge reads the app's bundle identifier and version from `Payload/*.app/Info.plist`, then returns the IPA path and the updated keychain/cookie state.
7. Swift persists rotated Anisette state and passes the prepared IPA directly to the existing local Telegraph server. The existing Palera HTTPS manifest endpoint and `itms-services` handoff remain in place. The server binds specifically to IPv4 loopback (`127.0.0.1`) to match the IPA URL in the manifest, and requests the available iOS background-execution window while the installer fetches the package.

## Files changed

- `ThirdParty/ipatool/bridge/ios/main.go` — exports `PancakeIPAToolDownload`.
- `ThirdParty/ipatool/bridge/ios/download.go` — implements native Store download, optional free-license acquisition, FairPlay preparation, and IPA metadata extraction.
- `ThirdParty/ipatool/bridge/ios/download_auth.go` — retries download/purchase once after a rejected password token.
- `ThirdParty/ipatool/bridge/ios/download_test.go` — tests IPA metadata extraction and invalid package handling.
- `ThirdParty/ipatool/bridge/ios/download_auth_test.go` — tests token-refresh retry behavior.
- `MuffinStoreJailed/bridge.h` — declares the new C bridge function.
- `MuffinStoreJailed/Functions/NativeIPAToolBridge.swift` — adds Swift request/response types and the C bridge call.
- `MuffinStoreJailed/Functions/IPATool.swift` — replaces the legacy Swift download/unzip/SINF path with the Go ipatool download and persists updated session state.
- `MuffinStoreJailed/Functions/Downgrader.swift` — serves the finished IPA directly; keeps the existing manifest/`itms-services` installation flow, binds to the matching IPv4 loopback address, and requests a background-execution window while iOS fetches the IPA.

## Verification and limitations

- `gofmt` completed on the changed Go sources.
- `swiftc -frontend -parse` completed on the changed Swift files.
- The Go unit tests and full iOS/Xcode build were not run in this environment. The available Go compiler is 1.23.2, while the project requires Go 1.26.5; the environment also has no Apple Xcode/iOS SDK. The repository's GitHub Actions workflow already provisions Go 1.26.5 and Xcode 26.6.
- This patch has not yet been tested on an iPhone. The first device test must verify the Apple download response, successful `sinf` preparation, manifest fetch, and the iOS installation prompt.
