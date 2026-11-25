# PancakeStore native version lookup: password-token refresh

## Problem

The native PancakeStore bridge called `AppStore.ListVersions()` once. When
Apple rejected the stored commerce token, ipatool returned
`ErrPasswordTokenExpired` and the bridge immediately stopped. The bundled
ipatool CLI already handles this condition by logging in again with the
stored account email/password and retrying the version lookup once.

## Change

- Added `listVersionsWithNativeRefresh` to the iOS Go bridge.
- On `ErrPasswordTokenExpired`, it performs one native `AppStore.Login()` with
'the email/password stored in the encrypted ipatool account keychain.
- It validates that Apple returned a complete account, then retries
  `ListVersions()` once using the new password token.
- The bridge's regular keychain/cookie export still runs after either success
  or failure, so refreshed Anisette state and cookies are returned to Swift.
- If no stored password is available, it preserves the original
  `ErrPasswordTokenExpired` identity and asks for sign-in.
- If Apple demands 2FA again, the bridge returns that authentication error
  rather than looping or silently discarding it.
- Added tests for success after refresh, unrelated errors, absent credentials,
  2FA-required refresh, and a second token rejection.

## Scope

No changes were made to Store endpoints, request payloads, the native
Anisette/AMD implementation, Unicorn/TCI, or the Swift version-list interface.

## Validation

- `gofmt` passed for all changed Go files.
- The new refresh helper and its five tests passed in an isolated Go test
  harness using stubbed ipatool API types.
- ZIP integrity and the workflow shell syntax passed.
- The full bundled ipatool test/build could not run: this environment has Go
  1.23.2, while the module declares Go 1.26.5; fetching the required toolchain
  is blocked by unavailable network access. The iOS Xcode/IPA build was not
  run on this non-macOS host.
