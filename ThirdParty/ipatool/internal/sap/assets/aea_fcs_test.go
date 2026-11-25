package assets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Apple FCS key requests", func() {
	const keyURL = "https://wkms-public.apple.com/fcs-keys/test-key"
	const pemKey = "-----BEGIN PRIVATE KEY-----\nYWJj\n-----END PRIVATE KEY-----\n"

	It("retries a transient network error and uses a browser-compatible user agent", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			Expect(request.URL.Host).To(Equal("wkms-public.apple.com"))
			Expect(request.UserAgent()).To(Equal("Mozilla/5.0"))
			if calls == 1 {
				return nil, io.EOF
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(pemKey)),
				Request:    request,
			}, nil
		})}

		data, err := downloadAppleFCSKey(context.Background(), client, keyURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(pemKey))
		Expect(calls).To(Equal(2))
	})

	It("retries a temporary gateway response", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				header := make(http.Header)
				header.Set("Retry-After", "0")
				return &http.Response{
					StatusCode: http.StatusGatewayTimeout,
					Status:     "504 Gateway Timeout",
					Header:     header,
					Body:       io.NopCloser(strings.NewReader("temporary gateway error")),
					Request:    request,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(pemKey)),
				Request:    request,
			}, nil
		})}

		data, err := downloadAppleFCSKey(context.Background(), client, keyURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(pemKey))
		Expect(calls).To(Equal(2))
	})

	It("does not retry a permanent authorization response", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Status:     "403 Forbidden",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("forbidden")),
				Request:    request,
			}, nil
		})}

		_, err := downloadAppleFCSKey(context.Background(), client, keyURL)
		Expect(err).To(MatchError(ContainSubstring("server returned 403 Forbidden")))
		Expect(calls).To(Equal(1))
	})

	It("stops immediately when the parent context is cancelled", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("unexpected request")
		})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := downloadAppleFCSKey(ctx, client, keyURL)
		Expect(err).To(MatchError(ContainSubstring(context.Canceled.Error())))
		Expect(calls).To(Equal(0))
	})
})
