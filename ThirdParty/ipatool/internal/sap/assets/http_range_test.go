package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

var _ = Describe("remote image HTTP requests", func() {
	It("reads image ranges from a server requiring a browser-compatible user agent", func() {
		const content = "restore image"

		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.UserAgent() != "Mozilla/5.0" {
				http.Error(response, "unsupported user agent", http.StatusForbidden)

				return
			}

			http.ServeContent(response, request, "restore.ipsw", time.Time{}, strings.NewReader(content))
		}))
		DeferCleanup(server.Close)

		transport := &http.Transport{}
		DeferCleanup(transport.CloseIdleConnections)

		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			request = request.Clone(request.Context())
			request.URL.Scheme = "http"
			request.URL.Host = server.Listener.Addr().String()

			return transport.RoundTrip(request)
		})}

		remote, err := openRemoteFile(context.Background(), client)
		Expect(err).NotTo(HaveOccurred())
		Expect(remote.size).To(Equal(int64(len(content))))
		remote.diskCache = nil

		data := make([]byte, len(content)-1)
		count, err := remote.ReadAt(data, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(len(data)))
		Expect(string(data)).To(Equal(content[1:]))
	})
})

var _ = Describe("remote image range retry behavior", func() {
	const content = "range response body"

	It("retries a transient connection failure with the same byte range", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("temporary network timeout")
			}
			return remoteRangeTestResponse(request, content), nil
		})}
		remote := &remoteFile{ctx: context.Background(), client: client, url: "https://restore.test/image.ipsw", size: int64(len(content))}
		data := make([]byte, len(content))

		count, err := remote.ReadAt(data, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(len(content)))
		Expect(string(data)).To(Equal(content))
		Expect(calls).To(Equal(2))
	})

	It("resumes an interrupted range body from the first unread byte", func() {
		calls := 0
		var requested []string
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			requested = append(requested, request.Header.Get("Range"))
			if calls == 1 {
				return remoteRangeTestResponseLimit(request, content, 5), nil
			}
			return remoteRangeTestResponse(request, content), nil
		})}
		remote := &remoteFile{ctx: context.Background(), client: client, url: "https://restore.test/image.ipsw", size: int64(len(content))}
		data := make([]byte, len(content))

		count, err := remote.ReadAt(data, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(len(content)))
		Expect(string(data)).To(Equal(content))
		Expect(requested).To(Equal([]string{"bytes=0-18", "bytes=5-18"}))
	})

	It("retries a temporary Apple CDN 5xx response", func() {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return &http.Response{
					StatusCode: http.StatusGatewayTimeout,
					Status:     "504 Gateway Timeout",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("temporary gateway error")),
					Request:    request,
				}, nil
			}
			return remoteRangeTestResponse(request, content), nil
		})}
		remote := &remoteFile{ctx: context.Background(), client: client, url: "https://restore.test/image.ipsw", size: int64(len(content))}
		data := make([]byte, len(content))

		count, err := remote.ReadAt(data, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(len(content)))
		Expect(string(data)).To(Equal(content))
		Expect(calls).To(Equal(2))
	})
})

func remoteRangeTestResponse(request *http.Request, content string) *http.Response {
	return remoteRangeTestResponseLimit(request, content, 0)
}

func remoteRangeTestResponseLimit(request *http.Request, content string, limit int) *http.Response {
	var start, end int
	if _, err := fmt.Sscanf(request.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
		panic(err)
	}
	body := content[start : end+1]
	contentLength := len(body)
	if limit > 0 && limit < len(body) {
		body = body[:limit]
	}
	header := make(http.Header)
	header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
	header.Set("Content-Length", strconv.Itoa(contentLength))
	return &http.Response{
		StatusCode: http.StatusPartialContent,
		Status:     "206 Partial Content",
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
