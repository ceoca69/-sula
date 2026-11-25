package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	remoteReadAttempts    = 5
	restoreImageUserAgent = "Mozilla/5.0"
	remoteSmallReadSize   = 32 << 10
	// Deflate readers used by archive/zip request small sequential chunks. A larger
	// read-ahead block avoids thousands of small HTTP Range requests while
	// extracting the pinned kernel collection from the remote IPSW.
	remoteReadAheadSize       = 8 << 20
	remoteRangeRequestTimeout = 45 * time.Second
	remoteRetryInitialDelay   = 500 * time.Millisecond
	remoteRetryMaximumDelay   = 4 * time.Second
)

type remoteFile struct {
	ctx         context.Context
	client      *http.Client
	url         string
	size        int64
	mu          sync.Mutex
	cache       []byte
	cacheOffset int64
	previous    bool
	lastEnd     int64
	diskCache   *remoteRangeCache
}

func newRemoteHTTPClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{}
	}

	transport := base.Clone()
	transport.TLSHandshakeTimeout = 20 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.IdleConnTimeout = 30 * time.Second
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 4
	transport.ForceAttemptHTTP2 = true
	transport.DisableCompression = true
	transport.DialContext = newRotatingIPDialer().DialContext

	return &http.Client{Transport: transport}
}

type rotatingIPDialer struct {
	resolver *net.Resolver
	dialer   net.Dialer
	mu       sync.Mutex
	next     int
}

func newRotatingIPDialer() *rotatingIPDialer {
	return &rotatingIPDialer{
		resolver: net.DefaultResolver,
		dialer: net.Dialer{
			Timeout:   8 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// DialContext rotates the first resolved address between reconnects. This is
// useful when an Apple CDN edge accepts TCP but then stalls a ranged response.
func (d *rotatingIPDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		return d.dialer.DialContext(ctx, network, address)
	}

	addresses, err := d.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s for remote IPSW: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("resolve %s for remote IPSW: no addresses", host)
	}

	d.mu.Lock()
	start := d.next % len(addresses)
	d.next = (d.next + 1) % len(addresses)
	d.mu.Unlock()

	var lastErr error
	for index := range addresses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		address := addresses[(start+index)%len(addresses)].IP
		transport := network
		if network == "tcp" {
			if address.To4() != nil {
				transport = "tcp4"
			} else {
				transport = "tcp6"
			}
		}
		if strings.HasSuffix(network, "4") && address.To4() == nil || strings.HasSuffix(network, "6") && address.To4() != nil {
			continue
		}
		connection, dialErr := d.dialer.DialContext(ctx, transport, net.JoinHostPort(address.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no addresses available for network %s", network)
	}
	return nil, fmt.Errorf("connect to %s: %w", host, lastErr)
}

//nolint:wsl // Request construction and validation are clearer in protocol order.
func openRemoteFile(ctx context.Context, client *http.Client) (*remoteFile, error) {
	var lastErr error

	for attempt := 0; attempt < remoteReadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("open remote image: %w", err)
		}

		requestContext, cancel := context.WithTimeout(ctx, remoteRangeRequestTimeout)
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, macOSRestoreURL, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create remote image request: %w", err)
		}

		request.Header.Set("User-Agent", restoreImageUserAgent)
		request.Header.Set("Range", "bytes=0-0")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			cancel()
			lastErr = requestErr
			client.CloseIdleConnections()
			if attempt+1 < remoteReadAttempts {
				if err := waitRemoteRetry(ctx, attempt, 0); err != nil {
					return nil, fmt.Errorf("open remote image: %w", err)
				}
				continue
			}
			break
		}

		if response.StatusCode != http.StatusPartialContent {
			status := response.Status
			_ = response.Body.Close()
			cancel()
			lastErr = fmt.Errorf("server returned %s without byte-range support", status)
			retryAfter := parseRemoteRetryAfter(response.Header.Get("Retry-After"))
			if !retryableRemoteStatus(response.StatusCode) || attempt+1 == remoteReadAttempts {
				return nil, fmt.Errorf("open remote image: %w", lastErr)
			}
			fmt.Printf("[ipatool/assets] retrying remote IPSW probe after HTTP %s attempt=%d/%d\n", status, attempt+1, remoteReadAttempts)
			client.CloseIdleConnections()
			if err := waitRemoteRetry(ctx, attempt, retryAfter); err != nil {
				return nil, fmt.Errorf("open remote image: %w", err)
			}
			continue
		}

		start, end, size, rangeErr := parseContentRange(response.Header.Get("Content-Range"))
		if rangeErr != nil || start != 0 || end != 0 {
			_ = response.Body.Close()
			cancel()
			if rangeErr != nil {
				return nil, fmt.Errorf("open remote image: %w", rangeErr)
			}
			return nil, fmt.Errorf("open remote image: server returned unexpected byte range %d-%d", start, end)
		}

		probe, readErr := io.ReadAll(io.LimitReader(response.Body, 2))
		closeErr := response.Body.Close()
		cancel()
		if readErr != nil || closeErr != nil || len(probe) != 1 {
			lastErr = errors.Join(readErr, closeErr)
			if lastErr == nil {
				lastErr = errors.New("remote image probe has an invalid length")
			}
			if attempt+1 == remoteReadAttempts {
				break
			}
			client.CloseIdleConnections()
			if err := waitRemoteRetry(ctx, attempt, 0); err != nil {
				return nil, fmt.Errorf("read remote image probe: %w", err)
			}
			continue
		}

		var diskCache *remoteRangeCache
		if cache, cacheErr := newRemoteRangeCache(macOSRestoreURL); cacheErr == nil {
			diskCache = cache
		}

		return &remoteFile{ctx: ctx, client: client, url: macOSRestoreURL, size: size, diskCache: diskCache}, nil
	}

	if lastErr == nil {
		lastErr = errors.New("remote image probe failed")
	}

	return nil, fmt.Errorf("open remote image after %d attempts: %w", remoteReadAttempts, lastErr)
}

//nolint:wsl // Parsing checks are clearer in wire-format order.
func parseContentRange(value string) (int64, int64, int64, error) {
	unitAndRange, total, ok := strings.Cut(value, "/")
	if !ok || total == "" || total == "*" {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range %q", value)
	}
	size, err := strconv.ParseInt(total, 10, 64)
	if err != nil || size <= 0 {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range %q", value)
	}
	rangeValue, ok := strings.CutPrefix(unitAndRange, "bytes ")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range %q", value)
	}

	startValue, endValue, ok := strings.Cut(rangeValue, "-")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range %q", value)
	}
	start, startErr := strconv.ParseInt(startValue, 10, 64)
	end, endErr := strconv.ParseInt(endValue, 10, 64)
	if startErr != nil || endErr != nil || start < 0 || end < start || end >= size {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range %q", value)
	}

	return start, end, size, nil
}

//nolint:wsl // Range request validation is clearer in protocol order.
func (r *remoteFile) ReadAt(data []byte, offset int64) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	if offset < 0 {
		return 0, errors.New("negative remote image offset")
	}

	if offset >= r.size {
		return 0, io.EOF
	}

	wanted := len(data)
	if remaining := r.size - offset; int64(wanted) > remaining {
		wanted = int(remaining)
	}

	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("read remote image: %w", err)
	}
	r.mu.Lock()
	if offset >= r.cacheOffset && offset+int64(wanted) <= r.cacheOffset+int64(len(r.cache)) {
		copy(data[:wanted], r.cache[offset-r.cacheOffset:offset-r.cacheOffset+int64(wanted)])
		r.previous, r.lastEnd = true, offset+int64(wanted)
		r.mu.Unlock()
		if wanted != len(data) {
			return wanted, io.EOF
		}

		return wanted, nil
	}
	// Sparse metadata reads stay exact; only a sequential small-read stream
	// earns read-ahead. This avoids fetching large unused image regions.
	ahead := r.previous && offset == r.lastEnd && wanted <= remoteSmallReadSize
	r.previous, r.lastEnd = true, offset+int64(wanted)
	r.mu.Unlock()
	fetchSize := wanted
	if ahead {
		fetchSize = int(min(int64(remoteReadAheadSize), r.size-offset))
	}
	buffer := data[:wanted]
	if fetchSize > wanted {
		buffer = make([]byte, fetchSize)
	}

	if r.diskCache != nil && r.diskCache.ReadAt(buffer, offset) {
		if fetchSize > wanted {
			copy(data[:wanted], buffer[:wanted])
		}
		r.mu.Lock()
		r.cacheOffset, r.cache = offset, bytes.Clone(buffer)
		r.mu.Unlock()
		if wanted != len(data) {
			return wanted, io.EOF
		}
		return wanted, nil
	}

	count, err := r.readRange(buffer, offset)
	if count == fetchSize && err == nil {
		if r.diskCache != nil {
			_ = r.diskCache.Store(buffer, offset)
		}
		cached := buffer
		if fetchSize == wanted {
			cached = bytes.Clone(buffer)
		}
		r.mu.Lock()
		r.cacheOffset, r.cache = offset, cached
		r.mu.Unlock()
	}
	if fetchSize > wanted {
		copy(data[:wanted], buffer[:min(count, wanted)])
	}
	if count < wanted {
		return count, err
	}
	if wanted != len(data) {
		return wanted, io.EOF
	}

	return wanted, nil
}

// Read and validate one wire range, resuming interrupted bodies as needed.
func (r *remoteFile) readRange(data []byte, offset int64) (int, error) {
	wanted := len(data)
	total := 0
	var lastErr error

	for attempt := 0; attempt < remoteReadAttempts && total < wanted; attempt++ {
		if err := r.ctx.Err(); err != nil {
			return total, fmt.Errorf("read remote image: %w", err)
		}

		start := offset + int64(total)
		end := offset + int64(wanted) - 1
		requestContext, cancel := context.WithTimeout(r.ctx, remoteRangeRequestTimeout)
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, r.url, nil)
		if err != nil {
			cancel()
			return total, fmt.Errorf("create remote range request: %w", err)
		}

		request.Header.Set("User-Agent", restoreImageUserAgent)
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

		response, requestErr := r.client.Do(request)
		if requestErr != nil {
			cancel()
			lastErr = requestErr
			fmt.Printf("[ipatool/assets] transient IPSW range request failure offset=%d attempt=%d/%d: %v\n", start, attempt+1, remoteReadAttempts, requestErr)
			r.client.CloseIdleConnections()
			if attempt+1 < remoteReadAttempts {
				if err := waitRemoteRetry(r.ctx, attempt, 0); err != nil {
					return total, fmt.Errorf("read remote image at offset %d: %w", offset+int64(total), err)
				}
				continue
			}
			break
		}

		if response.StatusCode != http.StatusPartialContent {
			statusCode, status := response.StatusCode, response.Status
			retryAfter := parseRemoteRetryAfter(response.Header.Get("Retry-After"))
			_ = response.Body.Close()
			cancel()
			lastErr = fmt.Errorf("server returned %s", status)
			if !retryableRemoteStatus(statusCode) || attempt+1 == remoteReadAttempts {
				return total, fmt.Errorf("read remote image range: %w", lastErr)
			}
			fmt.Printf("[ipatool/assets] retrying IPSW range after HTTP %s offset=%d attempt=%d/%d\n", status, start, attempt+1, remoteReadAttempts)
			r.client.CloseIdleConnections()
			if err := waitRemoteRetry(r.ctx, attempt, retryAfter); err != nil {
				return total, fmt.Errorf("read remote image at offset %d: %w", offset+int64(total), err)
			}
			continue
		}

		actualStart, actualEnd, actualSize, rangeErr := parseContentRange(response.Header.Get("Content-Range"))
		if rangeErr != nil || actualStart != start || actualEnd != end || actualSize != r.size {
			_ = response.Body.Close()
			cancel()
			if rangeErr != nil {
				return total, fmt.Errorf("read remote image range: %w", rangeErr)
			}
			return total, fmt.Errorf("read remote image range: server returned bytes %d-%d/%d, expected %d-%d/%d", actualStart, actualEnd, actualSize, start, end, r.size)
		}

		count, readErr := io.ReadFull(response.Body, data[total:wanted])
		_ = response.Body.Close()
		cancel()
		total += count
		if readErr == nil {
			lastErr = nil
			break
		}

		lastErr = readErr
		fmt.Printf("[ipatool/assets] interrupted IPSW range body offset=%d received=%d/%d attempt=%d/%d: %v\n", start, count, wanted-total+count, attempt+1, remoteReadAttempts, readErr)
		r.client.CloseIdleConnections()
		if attempt+1 < remoteReadAttempts {
			if err := waitRemoteRetry(r.ctx, attempt, 0); err != nil {
				return total, fmt.Errorf("read remote image at offset %d: %w", offset+int64(total), err)
			}
		}
	}

	if total != wanted {
		if lastErr == nil {
			lastErr = io.ErrUnexpectedEOF
		}
		return total, fmt.Errorf("read remote image at offset %d after %d attempts: %w", offset+int64(total), remoteReadAttempts, lastErr)
	}

	return total, nil
}

func retryableRemoteStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseRemoteRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		if delay := time.Until(date); delay > 0 {
			return delay
		}
	}
	return 0
}

func waitRemoteRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := remoteRetryInitialDelay << min(attempt, 3)
	if delay > remoteRetryMaximumDelay {
		delay = remoteRetryMaximumDelay
	}
	if retryAfter > delay {
		delay = min(retryAfter, 10*time.Second)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
