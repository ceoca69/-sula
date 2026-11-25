# PancakeStore LE v1.0 — Apple FCS key retry fix

Date: 2026-11-24

## Root cause

First-time native Store authentication prepares Anisette assets before it submits the Apple ID authentication request. During AEA filesystem decryption, `resolveAEASymmetricKey` downloads the Apple-published FCS PEM key from `wkms-public.apple.com`. The previous implementation issued one HTTP request and returned immediately on a transient network failure or timeout. The IPSW range reader already had retries, but the FCS-key request did not.

This matches upstream ipatool issue #671 (FCS key download not retried) and is distinct from the later 2FA handoff. A saved App Store session can bypass fresh native login, which explains why accounts with an existing session can work without triggering this first-run asset preparation.

## Changes

- Added `downloadAppleFCSKey` in `ThirdParty/ipatool/internal/sap/assets/aea.go`.
- Retries transient connection failures, response-body read failures, and retryable HTTP statuses using the existing range retry policy (`remoteReadAttempts`, backoff, and `Retry-After`).
- Gives each key request a 30-second deadline so a stalled WKMS request cannot wait indefinitely.
- Sends `User-Agent: Mozilla/5.0` on the FCS-key request.
- Retries after the initial configured transport using a fresh HTTP/1.1 transport clone with keep-alive disabled. This avoids reusing the same stalled HTTP/2 connection and allows the existing rotating dialer to select another resolved IP on a new connection.
- Does not retry permanent responses such as HTTP 403, and stops immediately when the parent context is cancelled.
- Added `aea_fcs_test.go` covering transient network retry, HTTP 504 retry, permanent 403 handling, user-agent, and context cancellation.

## Files changed

1. `ThirdParty/ipatool/internal/sap/assets/aea.go`
2. `ThirdParty/ipatool/internal/sap/assets/aea_fcs_test.go` (new)

## Validation

- `gofmt` completed on both changed Go files.
- Full Go tests could not run: this project requires Go 1.26.5, while the environment has Go 1.23.2 and could not resolve `proxy.golang.org` to download the required toolchain.
- No device-side Apple login or Xcode IPA build was performed in this environment.
