package rustygo

import (
	"context"
	"errors"
	"io"
	"net/http"
)

var (
	ErrResponseBodyTooLarge = errors.New("rustygo/client: response body exceeds maximum allowed bytes")
)

const defaultMaxBodyBytes = 32 * 1024 * 1024 // 32 MB

// ReadResponseBody reads the HTTP response body directly into an arena Scope.
// Completely bypasses Go standard library io.ReadAll heap allocations.
func ReadResponseBody(resp *http.Response, scope *Scope, maxBytes int64) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}

	initialCap := int(resp.ContentLength)
	if initialCap <= 0 || initialCap > 1024*1024 {
		initialCap = 4096
	}

	buf := NewArenaBuffer(scope, initialCap)
	tmp := make([]byte, 8192)

	var totalRead int64
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			totalRead += int64(n)
			if totalRead > maxBytes {
				return nil, ErrResponseBodyTooLarge
			}
			_, _ = buf.Write(tmp[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Client wraps an http.Client providing arena-backed request execution.
type Client struct {
	HTTPClient *http.Client
	MaxBody    int64
}

// DefaultClient is the package default arena-aware HTTP client.
var DefaultClient = &Client{
	HTTPClient: http.DefaultClient,
	MaxBody:    defaultMaxBodyBytes,
}

// GetScope executes an HTTP GET request and stores the response body directly in scope.
func (c *Client) GetScope(ctx context.Context, url string, scope *Scope) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	return c.DoScope(req, scope)
}

// DoScope executes the request and streams the response directly into the specified Scope.
func (c *Client) DoScope(req *http.Request, scope *Scope) ([]byte, *http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	body, err := ReadResponseBody(resp, scope, c.MaxBody)
	if err != nil {
		return nil, resp, err
	}

	return body, resp, nil
}
