# Native ipatool version lookup bridge — 2026-11-23

## Problem

PancakeStore's downgrade version-history lookup called the legacy Swift
`StoreClient.download()` method. That method sent a direct `URLSession` request
with JSON bytes and `Content-Type: application/x-www-form-urlencoded`, bypassing
the current ipatool Store commerce request path.

The current embedded ipatool implementation already provides
`AppStore.ListVersions()`. It routes the request through `sendDownloadProduct()`
and `sendCommerceDownloadRequest()`, where Store AMD proof headers are generated
from the persisted native Anisette state and attached to allowlisted commerce
requests. It also reuses the native keychain and cookie jar.

## Changes

1. `ThirdParty/ipatool/bridge/ios/main.go`
   - Added `PancakeIPAToolListVersions` C export.
   - Restores the native keychain/cookies and stored account.
   - Calls `AppStore.ListVersions()` for the requested iOS app ID.
   - Returns version identifiers, updated keychain state, and updated cookies.
   - Uses a five-minute request context and serializes bridge calls with login.

2. `MuffinStoreJailed/bridge.h`
   - Declares the new C bridge function.

3. `MuffinStoreJailed/Functions/NativeIPAToolBridge.swift`
   - Adds request/response models and a Swift wrapper for the new C export.

4. `MuffinStoreJailed/Functions/IPATool.swift`
   - Routes `getVersionIDList()` through the embedded ipatool bridge rather than
     the legacy direct Swift download request.
   - Persists updated native Anisette/keychain and cookie state returned by Go.
   - Redacts token/DSID/cookie values from the old HTTP header debug log.

## Verification

- Swift frontend syntax parsing passed for the two changed Swift files.
- `gofmt` parsing/format check passed for the changed Go bridge file.
- ZIP integrity checked after packaging.
- Full Go/iOS build and device-side Store request were not run in this
  environment. The repository requires Go 1.26.5; this environment has Go
  1.23.2 and could not download the required toolchain.

The device test should first verify that the version IDs are returned. The
pinned IPA download function remains on the legacy Swift request path and is a
separate follow-up task after version listing is confirmed.
