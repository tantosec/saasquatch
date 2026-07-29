// pkg/requester/requester_test.go
package requester

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tantosec/saasquatch/internal/client"
	"github.com/tantosec/saasquatch/internal/rules"
)

func TestRequestFromRule(t *testing.T) {
	client.InitClient(10*time.Second, "", 1024, false)

	var capturedUserAgent string
	var mu sync.Mutex // Mutex to protect access to capturedUserAgent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedUserAgent = r.Header.Get("User-Agent")
		mu.Unlock()

		if r.URL.Path == "/proxy-error" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "<h1>Burp Suite Professional</h1><p>Proxy error page content.</p>")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Base rule request to be modified in each sub-test
	baseRuleReq := rules.Request{
		Method:   "GET",
		Protocol: "http",
	}

	t.Run("Custom User-Agent", func(t *testing.T) {
		reqCfg := RequesterConfig{
			IdentifierPlaceholder: "IDENTIFIER",
			UserAgent:             "MyCustomAgent/1.0",
		}
		ruleReq := baseRuleReq
		ruleReq.Path = server.Listener.Addr().String() + "/custom"

		_, err := RequestFromRule(reqCfg, ruleReq, "test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if capturedUserAgent != "MyCustomAgent/1.0" {
			t.Errorf("Expected User-Agent 'MyCustomAgent/1.0', but got '%s'", capturedUserAgent)
		}
	})

	t.Run("Random User-Agent from file", func(t *testing.T) {
		tempDir := t.TempDir()
		uaFile := filepath.Join(tempDir, "ua.txt")
		uaContent := "Mozilla/5.0\nChrome/100.0"
		if err := os.WriteFile(uaFile, []byte(uaContent), 0644); err != nil {
			t.Fatalf("Failed to write ua file: %v", err)
		}

		reqCfg := RequesterConfig{
			IdentifierPlaceholder: "IDENTIFIER",
			UserAgent:             "saasquatch", // Default is overridden by random agent file
			RandomAgentFile:       uaFile,
		}
		ruleReq := baseRuleReq
		ruleReq.Path = server.Listener.Addr().String() + "/random"

		_, err := RequestFromRule(reqCfg, ruleReq, "test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if capturedUserAgent != "Mozilla/5.0" && capturedUserAgent != "Chrome/100.0" {
			t.Errorf("Expected a random User-Agent from file, but got '%s'", capturedUserAgent)
		}
	})

	t.Run("Proxy error page detection", func(t *testing.T) {
		reqCfg := RequesterConfig{
			IdentifierPlaceholder: "IDENTIFIER",
			ProxyErrorStrings:     []string{"<h1>Burp Suite Professional</h1>"},
			ProxyInUse:            true,
		}
		ruleReq := baseRuleReq
		ruleReq.Path = server.Listener.Addr().String() + "/proxy-error"

		_, err := RequestFromRule(reqCfg, ruleReq, "test")
		if err == nil {
			t.Fatal("Expected an error due to proxy error page detection, but got none")
		}
		if !strings.Contains(err.Error(), "matches a configured proxy error page") {
			t.Errorf("Expected error to contain proxy message, but got: %v", err)
		}
	})

	t.Run("Proxy error page scan skipped without proxy", func(t *testing.T) {
		reqCfg := RequesterConfig{
			IdentifierPlaceholder: "IDENTIFIER",
			ProxyErrorStrings:     []string{"<h1>Burp Suite Professional</h1>"},
			ProxyInUse:            false,
		}
		ruleReq := baseRuleReq
		ruleReq.Path = server.Listener.Addr().String() + "/proxy-error"

		resp, err := RequestFromRule(reqCfg, ruleReq, "test")
		if err != nil {
			t.Fatalf("Expected no error when no proxy is in use, got: %v", err)
		}
		resp.Body.Close()
	})
}
