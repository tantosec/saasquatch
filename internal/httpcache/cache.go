package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/tantosec/saasquatch/internal/logging"
)

type CachingRoundTripper struct {
	Transport http.RoundTripper
	Cache     *lru.Cache[string, cachedResponse]
	KeyGen    KeyGenerator
}

type KeyGenerator func(req *http.Request) string

type cachedResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func NewCachingRoundTripper(size int, nextTransport http.RoundTripper) (*CachingRoundTripper, error) {
	if size <= 0 {
		return nil, fmt.Errorf("cache size must be a positive integer")
	}

	lruCache, err := lru.New[string, cachedResponse](size)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %v", err)
	}

	return &CachingRoundTripper{
		Transport: nextTransport,
		Cache:     lruCache,
		KeyGen:    DefaultKeyGenerator,
	}, nil
}

func (crt *CachingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	logger := logging.GetLogger()
	key := crt.KeyGen(req)

	if cachedVal, ok := crt.Cache.Get(key); ok {
		logger.Debug("💥 Cache Hit", "URL", req.URL)

		// Reconstruct the response from the cached data.
		resp := &http.Response{
			StatusCode: cachedVal.StatusCode,
			Header:     cachedVal.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(cachedVal.Body)),
			Request:    req,
		}
		resp.Header.Set("X-SaaSquatch-Cache-Status", "HIT")
		return resp, nil
	}

	// If not in cache, make the actual network request.
	resp, err := crt.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if isCacheable(resp.StatusCode) {
		crt.Cache.Add(key, cachedResponse{
			StatusCode: resp.StatusCode,
			Header:     resp.Header.Clone(),
			Body:       body,
		})
	} else {
		logger.Debug("⏭️  Skipping cache for transient error", "URL", req.URL, "status", resp.StatusCode)
	}

	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
	newResp.Header.Set("X-SaaSquatch-Cache-Status", "MISS")
	return newResp, nil
}

func isCacheable(statusCode int) bool {
	return statusCode < 500 && statusCode != http.StatusTooManyRequests
}

func DefaultKeyGenerator(req *http.Request) string {
	var sb strings.Builder
	sb.WriteString(req.Method)
	sb.WriteString(":")
	sb.WriteString(req.URL.String())
	return sb.String()
}
