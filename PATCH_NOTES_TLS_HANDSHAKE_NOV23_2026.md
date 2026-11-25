# PancakeStore TLS handshake diagnostic/fix — 2026-11-23

## What the supplied log establishes

The SEP/keychain operations and the Swift → C → Go bridge completed. `ipatool AppStore.Login` started the Store bag request, but Go's `net/http` transport returned `TLS handshake timeout` while connecting to `init.itunes.apple.com`. No HTTP response was received, so Store Bag parsing and credential validation were not reached.

## Changes

- Increased `http.Transport.TLSHandshakeTimeout` from Go's default 10 seconds to 45 seconds for both the normal and authentication transports (the authentication transport is cloned from the configured transport).
- Added `httptrace` diagnostics around DNS resolution, TCP connect, TLS handshake, connection acquisition, and first response byte. Logs avoid printing URL query parameters, cookies, or GUIDs.
- Added a regression test asserting that both transports use the 45-second handshake timeout.

## Important limitation

This is a targeted transport adjustment plus instrumentation, not a proven authentication fix. If the handshake still does not complete within 45 seconds, the new trace will show whether the delay is in DNS, TCP connect, or TLS itself. A timeout at that point may indicate network/VPN/filtering/routing or TLS compatibility, rather than a too-short timeout.

## Build verification

The source was gofmt-formatted. Full Go tests/build could not be run in this environment because the installed Go is 1.23.2 while the repository's `go.mod` requires Go 1.26.5. The iOS/Xcode build must be run in the project's macOS workflow.
