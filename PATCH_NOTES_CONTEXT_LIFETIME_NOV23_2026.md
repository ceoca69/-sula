# PancakeStore — Store Bag response-context fix (23 Nov 2026)

## Current issue

The latest device log showed that DNS, TCP, TLS and HTTP/2 setup all completed, and the server's first response byte arrived. Reading the response then failed with `context canceled`.

## Root cause

`ThirdParty/ipatool/internal/store/backend.go`, in `executor.Send`, wrapped every request in `context.WithTimeout` and deferred `cancel()`. The `bag-direct` path returns a live `*http.Response` from that function. Its caller (`pkg/http.client.Send`) reads `Response.Body` only after `executor.Send` has returned. The deferred cancellation therefore terminated the request context before the response body could be consumed.

Go's `net/http` documentation confirms that the request context controls the whole outgoing request lifetime, including reading response headers and body. `http.Client.Timeout` likewise covers response-body reads and remains active after `Do` returns.

## Changes

- `ThirdParty/ipatool/internal/store/backend.go` — do not create/defer-cancel an extra timeout context for the `bag-direct` operation. Its `requestTransport` already configures `http.Client.Timeout` from `request.Timeout`; that timeout covers connection setup, response headers, and body consumption. Other operations retain their existing `context.WithTimeout` behavior because they execute the Store framework before returning.
- `ThirdParty/ipatool/internal/store/backend_test.go` — added regression tests proving a delayed body can be read after `Send` returns and that the configured timeout still interrupts a response body that never completes.

## Preserved

The prior Apple Bag TLS fallback, IP-address retry, HTTP/2, certificate verification, and 60-second Bag timeout are unchanged.

## Verification limits

The edited Go files were formatted with `gofmt`. Full Go tests could not be executed in this environment: the repository requires Go 1.26.5, and the Go toolchain/dependencies could not be downloaded because this environment could not resolve `proxy.golang.org`. Build and run the GitHub Actions workflow before treating the IPA as verified.
