package client

import (
	"net"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/tantosec/saasquatch/internal/httpcache"
)

func stubProxyListener(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start stub proxy listener: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

func resetSingleton() {
	once = sync.Once{}
	globalClient = nil
}

func TestInitClient(t *testing.T) {
	// Test case 1: Basic initialization (no changes needed)
	t.Run("Basic Initialization", func(t *testing.T) {
		resetSingleton()
		InitClient(10*time.Second, "", 1024, false)
		if GetClient() == nil {
			t.Fatal("Expected client to be initialized, but it was nil")
		}
	})

	// Test case 2: Initialization with HTTP proxy
	t.Run("Initialization with HTTP Proxy", func(t *testing.T) {
		resetSingleton()
		proxyURL := "http://" + stubProxyListener(t)
		InitClient(10*time.Second, proxyURL, 1024, false)
		client := GetClient()
		transport, _ := client.Transport.(*httpcache.CachingRoundTripper).Transport.(*http.Transport)

		// The Proxy function should NOT be the default one anymore.
		// We can check this by comparing the function pointers.
		defaultProxyFunc := reflect.ValueOf(http.ProxyFromEnvironment)
		currentProxyFunc := reflect.ValueOf(transport.Proxy)
		if currentProxyFunc.Pointer() == defaultProxyFunc.Pointer() {
			t.Error("Expected transport.Proxy to be set to a custom func, but it was still the default")
		}
	})

	// Test case 3: Initialization with SOCKS5 proxy
	t.Run("Initialization with SOCKS5 Proxy", func(t *testing.T) {
		resetSingleton()
		proxyURL := "socks5://" + stubProxyListener(t)
		InitClient(10*time.Second, proxyURL, 1024, false)
		client := GetClient()
		transport, _ := client.Transport.(*httpcache.CachingRoundTripper).Transport.(*http.Transport)

		// The Proxy function SHOULD be the default one, because we set DialContext instead.
		defaultProxyFunc := reflect.ValueOf(http.ProxyFromEnvironment)
		currentProxyFunc := reflect.ValueOf(transport.Proxy)
		if currentProxyFunc.Pointer() != defaultProxyFunc.Pointer() {
			t.Error("Expected transport.Proxy to be the default ProxyFromEnvironment for SOCKS5, but it was changed")
		}

		// The DialContext function should NOT be the default one anymore.
		defaultDialer := reflect.ValueOf((&http.Transport{}).DialContext)
		currentDialer := reflect.ValueOf(transport.DialContext)
		if currentDialer.Pointer() == defaultDialer.Pointer() {
			t.Error("Expected transport.DialContext to be set to the SOCKS dialer, but it was still the default")
		}
	})

	// Test case 4: Singleton behaviour (no changes needed)
	t.Run("Singleton Behavior", func(t *testing.T) {
		resetSingleton()
		timeout := 20 * time.Second
		InitClient(timeout, "", 10, false)
		firstClient := GetClient()
		InitClient(5*time.Second, "http://proxy.invalid", 50, false)
		secondClient := GetClient()
		if firstClient != secondClient {
			t.Error("Expected GetClient to return the same instance on multiple calls")
		}
		if secondClient.Timeout != timeout {
			t.Errorf("Expected timeout to remain %v from first initialization, but got %v", timeout, secondClient.Timeout)
		}
	})
}
