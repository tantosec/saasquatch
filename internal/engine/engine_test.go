// engine/engine_test.go
package engine

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tantosec/saasquatch/internal/client"
	"github.com/tantosec/saasquatch/internal/config"
	"github.com/tantosec/saasquatch/internal/rules"
)

func TestExtractCaptures(t *testing.T) {
	newResp := func(header http.Header, body string) *http.Response {
		return &http.Response{Header: header, Body: io.NopCloser(strings.NewReader(body))}
	}

	testCases := []struct {
		name       string
		captures   map[string]rules.Capture
		header     http.Header
		body       string
		identifier string
		want       map[string]any
	}{
		{
			name:     "body single match group",
			captures: map[string]rules.Capture{"tenant": {Regex: `data-tenant="([0-9a-f-]+)"`}},
			body:     `<div data-tenant="a1b2-c3d4">`,
			want:     map[string]any{"tenant": "a1b2-c3d4"},
		},
		{
			name:     "all matches list",
			captures: map[string]rules.Capture{"emails": {Regex: `([\w.]+@example\.com)`, All: true}},
			body:     `a@example.com and b@example.com`,
			want:     map[string]any{"emails": []string{"a@example.com", "b@example.com"}},
		},
		{
			name:     "header location",
			captures: map[string]rules.Capture{"sso": {Regex: `Location: (https://\S+)`}},
			header:   http.Header{"Location": {"https://login.example.com/sso"}},
			want:     map[string]any{"sso": "https://login.example.com/sso"},
		},
		{
			name:     "multiline body dotall",
			captures: map[string]rules.Capture{"para": {Regex: `(?s)<p>\s*(.*?)\s*</p>`}},
			body:     "<p>\nUNPREDICTABLE TEXT\n</p>",
			want:     map[string]any{"para": "UNPREDICTABLE TEXT"},
		},
		{
			name:     "no match omits key",
			captures: map[string]rules.Capture{"missing": {Regex: `nope-(\d+)`}},
			body:     `nothing here`,
			want:     map[string]any{},
		},
		{
			name:       "identifier placeholder substituted",
			captures:   map[string]rules.Capture{"id": {Regex: `IDENTIFIER-(\d+)`}},
			body:       `acme-42`,
			identifier: "acme",
			want:       map[string]any{"id": "42"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.header
			if header == nil {
				header = http.Header{}
			}
			got := extractCaptures(tc.captures, newResp(header, tc.body), tc.identifier, "IDENTIFIER")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestProxyInUse(t *testing.T) {
	proxyEnv := []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"}

	t.Run("explicit proxy", func(t *testing.T) {
		if !proxyInUse(&config.AppConfig{Proxy: "http://127.0.0.1:8080"}) {
			t.Error("explicit --proxy should count as in use")
		}
	})
	t.Run("no proxy, no env", func(t *testing.T) {
		for _, k := range proxyEnv {
			t.Setenv(k, "")
		}
		if proxyInUse(&config.AppConfig{}) {
			t.Error("no proxy and no env should be false")
		}
	})
	t.Run("env proxy honored", func(t *testing.T) {
		for _, k := range proxyEnv {
			t.Setenv(k, "")
		}
		t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8080")
		if !proxyInUse(&config.AppConfig{}) {
			t.Error("env proxy should count as in use")
		}
	})
	t.Run("env proxy ignored", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8080")
		if proxyInUse(&config.AppConfig{IgnoreEnvProxies: true}) {
			t.Error("--ignore-env-proxies should override env proxy")
		}
	})
}

func TestHostDoesNotExist(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"dns not found", &net.DNSError{IsNotFound: true}, true},
		{"dns not found wrapped", fmt.Errorf("request failed: %w", &net.DNSError{IsNotFound: true}), true},
		{"dns timeout", &net.DNSError{IsTimeout: true}, false},
		{"generic error", errors.New("connection refused"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostDoesNotExist(tc.err); got != tc.want {
				t.Errorf("hostDoesNotExist(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

var testCfg = &config.AppConfig{
	IdentifierPlaceholder: "IDENTIFIER",
	Timeout:               5,
	Proxy:                 "",
	LRUCacheSize:          10,
}

// TestProcessRule tests the core logic of checking a single identifier against a rule.
func TestProcessRule(t *testing.T) {
	// This advanced handler simulates various web service responses based on the request path.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Status check endpoint
		if strings.HasPrefix(r.URL.Path, "/status") {
			if strings.Contains(r.URL.Path, "good-identifier") {
				w.WriteHeader(http.StatusOK) // 200
			} else {
				w.WriteHeader(http.StatusNotFound) // 404
			}
			return
		}

		// Header check endpoint
		if strings.HasPrefix(r.URL.Path, "/header") {
			if strings.Contains(r.URL.Path, "good-identifier") {
				w.Header().Set("Location", "/users/dashboard")
			} else {
				w.Header().Set("Location", "/users/login_failed")
			}
			w.WriteHeader(http.StatusFound) // 302
			return
		}

		// Body check endpoint
		if strings.HasPrefix(r.URL.Path, "/body") {
			if strings.Contains(r.URL.Path, "good-identifier") {
				fmt.Fprint(w, "Welcome to the organization dashboard.")
			} else {
				fmt.Fprint(w, "Sorry, that organization was not found.")
			}
			return
		}

		// Default fallback
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse test server URL: %v", err)
	}

	// Initialize the global HTTP client once for all tests in this file.
	client.InitClient(time.Duration(testCfg.Timeout)*time.Second, testCfg.Proxy, testCfg.LRUCacheSize, testCfg.IgnoreEnvProxies)

	// Define table-driven tests
	testCases := []struct {
		name             string
		identifierToTest string
		rule             rules.Rule
		expectFound      bool
		expectErr        bool
	}{
		{
			name:             "Status Match - Positive - Should Find",
			identifierToTest: "good-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/status/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Status: []int{200, 201}}},
				Condition: "match",
			},
			expectFound: true,
		},
		{
			name:             "Status Match - Positive - Should Not Find",
			identifierToTest: "bad-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/status/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Status: []int{200}}},
				Condition: "match",
			},
			expectFound: false,
		},
		{
			name:             "Status Match - Negative - Should Find",
			identifierToTest: "bad-identifier", // A bad identifier returns 404
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/status/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Status: []int{200}}},
				Condition: "not match", // Rule succeeds if status is NOT 200
			},
			expectFound: true,
		},
		{
			name:             "Header Regex Match - Positive - Should Find",
			identifierToTest: "good-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/header/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Header: map[string][]string{"Location": {"/users/dashboard"}}}},
				Condition: "match",
			},
			expectFound: true,
		},
		{
			name:             "Header Regex Match - Negative - Should Find",
			identifierToTest: "bad-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/header/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Header: map[string][]string{"Location": {"/users/dashboard"}}}},
				Condition: "not match", // Succeeds if this is not found
			},
			expectFound: true,
		},
		{
			name:             "Body Regex Match - Positive - Should Find",
			identifierToTest: "good-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/body/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Body: []string{"welcome to the organization"}}},
				Condition: "match",
			},
			expectFound: true,
		},
		{
			name:             "Body Regex Match - Case Sensitive - Should Not Find",
			identifierToTest: "good-identifier",
			rule: rules.Rule{
				Request:   rules.Request{Method: "GET", Path: fmt.Sprintf("%s/body/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{"match": {Body: []string{"Welcome to the Organization"}, CaseSensitive: true}},
				Condition: "match",
			},
			expectFound: false, // Fails because of "Organization" capitalization
		},
		{
			name:             "Composed condition - (a or b) and not c - Should Find",
			identifierToTest: "good-identifier",
			rule: rules.Rule{
				Request: rules.Request{Method: "GET", Path: fmt.Sprintf("%s/header/IDENTIFIER", serverURL.Host), Protocol: "http"},
				Detection: map[string]rules.Leaf{
					"a": {Status: []int{302}},
					"b": {Status: []int{200}},
					"c": {Header: map[string][]string{"Location": {"/users/login_failed"}}},
				},
				Condition: "(a or b) and not c",
			},
			expectFound: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := rules.CompileCondition(tc.rule.Condition, tc.rule.Detection)
			if err != nil {
				t.Fatalf("Failed to compile condition %q: %v", tc.rule.Condition, err)
			}
			tc.rule.Cond = prog

			found, _, err := processRule(testCfg, tc.rule, tc.identifierToTest)

			if tc.expectErr {
				if err == nil {
					t.Error("Expected an error but got none")
				}
			} else if err != nil {
				t.Errorf("Did not expect an error, but got: %v", err)
			}

			if found != tc.expectFound {
				t.Errorf("Expected found status to be %v, but got %v", tc.expectFound, found)
			}
		})
	}
}
func TestGetIdentifiers_FromStdin(t *testing.T) {

	testCases := []struct {
		name                string
		input               string
		expectedIdentifiers []string
	}{
		{
			name:                "Standard Input with extra whitespace and empty lines",
			input:               "identifier1\n  identifier2  \n\nidentifier3",
			expectedIdentifiers: []string{"identifier1", "identifier2", "identifier3"},
		},
		{
			name:                "Input with internal whitespace in identifiers",
			input:               "google\n  my company  \n\nanother one",
			expectedIdentifiers: []string{"google", "my company", "another one"},
		},
		{
			name:                "Empty Input",
			input:               "",
			expectedIdentifiers: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			r, w, _ := os.Pipe()

			originalStdin := os.Stdin
			os.Stdin = r
			defer func() { os.Stdin = originalStdin }()

			go func() {
				defer w.Close()
				w.WriteString(tc.input)
			}()

			identifiers, err := getIdentifiers("", "")
			if err != nil {
				t.Fatalf("getIdentifiers returned an unexpected error: %v", err)
			}

			if !reflect.DeepEqual(identifiers, tc.expectedIdentifiers) {
				t.Errorf("Expected identifiers %v, but got %v", tc.expectedIdentifiers, identifiers)
			}
		})
	}
}
