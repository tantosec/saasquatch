package requester

import (
	"strings"
	"testing"
)

func TestBuildRequestURL(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		path       string
		port       int
		identifier string
		want       string
		wantError  string
	}{
		{
			name:     "substitutes identifier into host path",
			protocol: "https",
			path:     "github.com/orgs/IDENTIFIER",
			want:     "https://github.com/orgs/acme",
		},
		{
			name:     "adds non-default request port",
			protocol: "http",
			path:     "IDENTIFIER.example.com/path",
			port:     8080,
			want:     "http://acme.example.com:8080/path",
		},
		{
			name:     "keeps explicit path port when request port is unset",
			protocol: "http",
			path:     "example.com:9090/IDENTIFIER",
			want:     "http://example.com:9090/acme",
		},
		{
			name:      "rejects http scheme in request path",
			protocol:  "http",
			path:      "http://0.0.0.0:9090/file.txt",
			wantError: "request.path must not include a URL scheme",
		},
		{
			name:      "rejects https scheme in request path",
			protocol:  "https",
			path:      "https://IDENTIFIER.example.com",
			wantError: "request.path must not include a URL scheme",
		},
		{
			name:      "rejects unsupported protocol",
			protocol:  "ftp",
			path:      "example.com/IDENTIFIER",
			wantError: "unsupported protocol",
		},
		{
			name:       "rejects invalid identifier",
			protocol:   "https",
			path:       "example.com/IDENTIFIER",
			identifier: "bad/identifier",
			wantError:  "contains disallowed characters",
		},
		{
			name:      "rejects request port when path already has port",
			protocol:  "http",
			path:      "example.com:9090/IDENTIFIER",
			port:      8080,
			wantError: "port set in both request.port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier := tt.identifier
			if identifier == "" {
				identifier = "acme"
			}

			got, err := BuildRequestURL(tt.protocol, tt.path, tt.port, "IDENTIFIER", identifier)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got URL %q", tt.wantError, got)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildRequestURL returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("BuildRequestURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
