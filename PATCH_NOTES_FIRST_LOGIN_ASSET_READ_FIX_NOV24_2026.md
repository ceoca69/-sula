# PancakeStore LE v1.0 — First-login Apple asset read fix

Date: 24 November 2026

## Root cause

The first-run native Store login provisions Anisette by loading Apple's pinned macOS restore-image assets. This path is substantially heavier than a normal cached login.

Two local implementation choices caused the fresh-login failure:

1. `appstore.nativeLogin()` imposed a two-minute deadline on the entire login. Remote IPSW asset extraction can take several minutes on slower links; once this deadline expired, the active range request returned `context deadline exceeded`, and the parent cancellation prevented subsequent retry attempts.
2. The remote ZIP reader used 256 KiB read-ahead for sequential small reads. `archive/zip`'s Deflate reader requests small chunks (commonly 4 KiB), so reading the pinned kernel collection could trigger many small HTTP range fetches.

The upstream ipatool discussion in issue #648 documents the same first-sign-in bottleneck: 4 KiB reads, around 45 KiB/s observed throughput, and a report that 8 MiB read-ahead blocks moved the kernel-collection stage forward quickly. Upstream `nativeLogin` also uses `context.Background()` instead of a two-minute overall deadline.

## Changes

- Removed the two-minute overall deadline from `nativeLogin`, matching upstream's long-running first-run asset-preparation behavior. Individual remote Range requests still have a 45-second deadline and bounded retry/resume logic; Store HTTP requests retain their own client timeouts.
- Increased sequential remote-read-ahead from 256 KiB to 8 MiB. Sparse metadata reads remain exact and do not trigger speculative read-ahead.
- Kept the persistent cache limits aligned: each cached range can be up to 8 MiB; total cache remains bounded at 256 MiB.
- Added phase logs before and after reading the pinned kernel collection so first-run progress is visible.
- Updated the read-ahead test to assert the reduced request count for a sequential stream; adjusted the speculative-tail fixture to respect the bounded source length.

## Files changed

- `ThirdParty/ipatool/internal/sap/assets/http_range.go`
- `ThirdParty/ipatool/internal/sap/assets/http_buffer_test.go`
- `ThirdParty/ipatool/internal/sap/assets/kernel.go`
- `ThirdParty/ipatool/pkg/appstore/appstore_native_login.go`

## Validation

- `gofmt` completed for all four modified Go files.
- Full `go test` / build not run: source requires Go 1.26.5, while this environment has Go 1.23.2.
- Device verification is still required, especially a first login with no existing Apple Store / Anisette state.

This patch does not change credentials, 2FA handling, Store endpoints, FCS-key retrieval, Bundle ID, UI, or app installation behavior.
