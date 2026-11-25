# PancakeStore native-auth hang fix (Nov 23, 2026)

## Symptoms addressed
The app log showed two consecutive `IPATool` initializations and two native-auth starts, followed by no result from the embedded Go call.

## Changes
- Added a shared SwiftUI `isAuthenticating` state. Both password and 2FA buttons now guard against duplicate invocation, disable while active, and show a progress indicator.
- Serialized calls entering the Go C bridge as a second line of defense against concurrent Store runtime creation.
- Added non-sensitive stage diagnostics at the Swift bridge, Go bridge, ipatool native login, Store bag, AppleMediaServices runtime setup, and emulated request execution.
- Fixed timeout propagation in `internal/store/backend.go`: the configured authentication timeout now creates a context before Unicorn executes the emulated Apple framework. Previously the timeout only applied later to HTTP transport, so a stalled emulator execution could leave the login appearing frozen indefinitely.
- Added a two-minute deadline around the outer Anisette/Store AMD provisioning sequence.

## Expected next log
The next run should show the last completed stage, for example:
- `[GoBridge] login request received`
- `[ipatool] nativeLogin: fetching Store bag`
- `[ipatool] nativeLogin: preparing Anisette provisioning`
- `[ipatool/store] opening AppleMediaServices runtime for authenticate`
- `[ipatool/store] runtime ready; executing authenticate request`

If a stage exceeds its deadline, the operation should return an error rather than wait indefinitely, provided the runtime reaches its existing context checks.

## Validation limits
- Go source was formatted with `gofmt`.
- Static source assertions and ZIP integrity were checked.
- This environment has Go 1.23.2, while this ipatool source requires Go 1.26.5; the iOS C-archive and IPA were not rebuilt here.
- No real Apple account login was performed. This is a hang-diagnostic/timeout fix, not confirmation that Apple authentication succeeds.
