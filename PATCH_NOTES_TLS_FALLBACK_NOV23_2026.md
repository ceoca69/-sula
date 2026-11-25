# PancakeStore Store Bag TLS fallback — 2026-11-23

## Problem isolated from the device log

The SEP/Keychain operations and the Swift → C → Go bridge completed. `AppStore.Login()` entered the Store Bag request but did not receive an HTTP response: the connection to `init.itunes.apple.com` stalled during TLS negotiation. This occurs before Bag parsing, native Store AMD proof generation, or account credential validation.

## Implemented changes

1. **Dedicated Bag-only transport.** The Store authentication transport is cloned, and a custom TLS dialer is installed only for `GET https://init.itunes.apple.com/bag.xml`. Other Store/API requests keep their existing transport.
2. **Alternative Apple edge addresses.** The dialer resolves the hostname, interleaves IPv4/IPv6 candidates while preserving resolver preference, and tries up to four addresses. Each candidate gets up to 5 seconds for TCP connect and 8 seconds for TLS handshake. A stalled TLS handshake is closed and the next resolved address is attempted.
3. **TLS security is retained.** The TLS config is cloned from the existing transport; hostname/SNI verification, system/custom root certificate validation, and ALPN (HTTP/2 + HTTP/1.1 where enabled) are retained. No certificate-verification bypass is used.
4. **Corrected outer timeout.** The actual Bag client timeout was 30 seconds, shorter than the previous 45-second transport handshake setting. It is now 60 seconds so the request context does not cut off the address fallback sequence prematurely. The native login flow already has a 2-minute overall deadline.
5. **Diagnostics.** The log now reports DNS duration/count, each candidate IP and family, TCP results, TLS results, and negotiated protocol. It does not print the Bag query, GUID, cookies, or account secrets.
6. **Tests.** Added tests for fallback after a stalled TLS peer, exact request scoping, transport cloning, and a real local HTTP/2 TLS exchange with certificate verification enabled.

## Existing Apple native-login support

The supplied source already contains Configurator 2.21 Store Bag endpoint support, native `auth.itunes.apple.com/auth/v1/native` routing, SAP signing, and Store AMD header handling. This change leaves that logic untouched because the reported failure occurs before the Bag is received.

## Verification and limitations

The four focused transport tests passed in an isolated Go 1.23.2 standard-library test module. The full ipatool module test command could not run in this environment: its `go.mod` requires Go 1.26.5 while the installed toolchain is Go 1.23.2. The iOS/Xcode IPA build also must run in the repository's macOS GitHub Actions workflow. Therefore this is a source-level targeted fix and regression-tested transport, not yet a device-verified login success.
