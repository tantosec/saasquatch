// lruwebcache/cache_test.go
package httpcache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCachingRoundTripper(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		fmt.Fprintf(w, "request #%d", requestCount)
	}))
	defer server.Close()

	// --- TEST SETUP ---
	// 1. Create the caching round tripper, telling it to use the default transport for real requests.
	cachingTransport, err := NewCachingRoundTripper(10, http.DefaultTransport)
	if err != nil {
		t.Fatalf("Failed to create caching round tripper: %v", err)
	}
	// 2. Create an http.Client that uses our caching layer.
	testClient := &http.Client{
		Transport: cachingTransport,
	}
	// --- END OF SETUP ---

	// --- First Request ---
	t.Logf("Making first request to %s", server.URL)
	resp1, err := testClient.Get(server.URL)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Errorf("Expected server to be hit 1 time, but got %d", requestCount)
	}

	// --- Second Request (should be a cache hit) ---
	t.Logf("Making second request to %s", server.URL)
	resp2, err := testClient.Get(server.URL)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// The request count should NOT have increased.
	if requestCount != 1 {
		t.Errorf("Expected server to still have been hit only 1 time, but got %d", requestCount)
	}
}

func TestCachingRoundTripperSkipsTransientErrors(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.WriteHeader(status)
			}))
			defer server.Close()

			cachingTransport, err := NewCachingRoundTripper(10, http.DefaultTransport)
			if err != nil {
				t.Fatalf("Failed to create caching round tripper: %v", err)
			}
			testClient := &http.Client{Transport: cachingTransport}

			for i := 1; i <= 2; i++ {
				resp, err := testClient.Get(server.URL)
				if err != nil {
					t.Fatalf("request %d failed: %v", i, err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}

			if requestCount != 2 {
				t.Errorf("Expected %d response to bypass cache and hit server 2 times, got %d", status, requestCount)
			}
		})
	}
}
