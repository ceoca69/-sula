# PancakeStore LE — Store 504 retry and branding update

Date: 2026-11-23

## Store commerce 504 handling

- Added bounded retry handling to the legacy Store commerce download request used by ipatool.
- Retries transient HTTP statuses: 408, 429, 500, 502, 503, and 504.
- Retries selected transient transport failures (timeouts, EOF/unexpected EOF, connection reset, and broken pipe).
- Maximum of three request attempts, with 1s/2s exponential backoff unless a valid `Retry-After` asks for a longer delay within the configured retry budget.
- Stops immediately for caller cancellation, authorization/permanent errors, and other non-retryable responses.
- Rebuilds per-attempt request headers and regenerates the Store AMD/SAP proof for each attempt; the selected app version and request payload remain unchanged.
- Preserves HTTP status, content type, Apple's correlation ID, response-body length, and `Retry-After` metadata for non-plist gateway responses. HTML gateway markup is no longer copied into the primary error message.
- Added unit tests for HTML 504 parsing, retry/success, retry limit, `Retry-After`, rate-limit metadata, and non-retryable responses.

## Branding

- Display name and bundle name: `PancakeStore LE`.
- Marketing version: `1.0`.
- Bundle identifier remains `com.jbdotparty.PancakeStore`.
- Startup log prefix and exported log filename now use `PancakeStore LE`.
- Updated the localized app title and product references in all 12 existing localizations.
- Added `Working Edition by ceoca` below the existing Logs footer, using the same Markdown-link treatment and footer text style; `ceoca` links to `https://t.me/usrlibexec`.

## Validation

- `gofmt` completed for all modified Go files.
- Swift parser check passed for the modified Swift files.
- `plutil -lint` passed for the app Info.plist, Xcode project, and all localized `.strings` files.
- Static checks confirmed the display name, version settings, and unchanged app bundle identifier.
- Go package tests and a full Xcode/iOS build could not be run in this environment. The repository requires Go 1.26.5; automatic toolchain download failed because `proxy.golang.org` was unreachable. Xcode is not installed in this environment.
