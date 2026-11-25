# PancakeStore LE — IPSW range-read reliability fix

Date: 24 November 2026

## Problem

Native Store login obtains the Apple Store bag successfully, then fails while preparing Anisette. The failing step extracts AppleMediaServices from the pinned macOS 27.0.1 (26A434) restore image hosted on `updates.cdn-apple.com`.

The remote IPSW reader used HTTP Range requests, but it had only three immediate attempts, no per-range deadline, no retry delay for transient CDN responses, and no persistence of successfully fetched ranges between login attempts. A stalled IPv6/CDN edge could therefore cause the same large restore-image read to fail repeatedly, while a new app attempt re-read previously successful ranges.

## Changes

- Use a dedicated HTTP transport for the restore image, with bounded TLS/header/idle timeouts and HTTP/2 support.
- Rotate the first resolved address when a new TCP connection is required. IPv4 and IPv6 remain supported; neither family is globally disabled.
- Apply a 45-second deadline to each individual byte-range request.
- Increase retry attempts from 3 to 5, with bounded exponential backoff.
- Retry transient transport failures and HTTP 408, 425, 429, 500, 502, 503 and 504 responses. Respect a bounded `Retry-After` value where supplied.
- Resume an interrupted response from the first unread byte instead of restarting the entire range.
- Disable transparent HTTP compression for raw byte-range reads.
- Persist successfully received ranges under the app's cache directory, keyed to the pinned restore-image URL. Cache is capped at 256 MiB and each cached range is SHA-256 checked before reuse. This lets the next login reuse already-read IPSW regions after an interrupted attempt without downloading the entire IPSW.
- Log range retry details without exposing account credentials or Store authentication data.

## Files changed

1. `ThirdParty/ipatool/internal/sap/assets/http_range.go`
2. `ThirdParty/ipatool/internal/sap/assets/http_range_cache.go` (new)
3. `ThirdParty/ipatool/internal/sap/assets/http_range_test.go`
4. `ThirdParty/ipatool/internal/sap/assets/http_range_cache_test.go` (new)
5. `ThirdParty/ipatool/internal/sap/assets/remote.go`

No Swift UI, bundle identifier, version, native Store authentication, version lookup, IPA download, or installation code was changed.

## Validation

- `gofmt` completed for all changed Go files.
- An isolated Go test harness using the production range-reader and cache source passed checks for transient network failure retry, HTTP 504 retry, resuming a partial response, persistent cache reuse, and rejection of corrupted cache data.
- The full ipatool package test suite and IPA build were not run: the repository requires Go 1.26.5; the available local toolchain is Go 1.23.2 and this environment could not download the required toolchain. Xcode is not available here.

This improves resilience to intermittent CDN stalls; it cannot guarantee availability if Apple's CDN or the user's network remains unreachable.
