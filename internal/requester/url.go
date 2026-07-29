package requester

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var pathSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

const maxIdentifierLen = 253

func ValidateIdentifier(identifier string) error {
	if identifier == "" {
		return fmt.Errorf("identifier is empty")
	}
	if len(identifier) > maxIdentifierLen {
		return fmt.Errorf("identifier exceeds maximum length of %d characters", maxIdentifierLen)
	}
	if !identifierPattern.MatchString(identifier) {
		return fmt.Errorf("identifier %q contains disallowed characters (allowed: letters, digits, '.', '-', '_')", identifier)
	}
	return nil
}

func BuildRequestURL(protocol, pathTemplate string, port int, placeholder, identifier string) (string, error) {
	scheme := strings.ToLower(protocol)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported protocol %q (must be 'http' or 'https')", protocol)
	}
	if pathTemplate == "" {
		return "", fmt.Errorf("rule has an empty request path")
	}
	if placeholder == "" {
		return "", fmt.Errorf("identifier placeholder is empty (check identifier-placeholder config)")
	}
	if pathSchemePattern.MatchString(pathTemplate) {
		return "", fmt.Errorf("request.path must not include a URL scheme; set request.protocol separately")
	}
	if err := ValidateIdentifier(identifier); err != nil {
		return "", err
	}

	fullURL := fmt.Sprintf("%s://%s", scheme, strings.ReplaceAll(pathTemplate, placeholder, identifier))

	parsed, err := url.Parse(fullURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse constructed URL %q: %w", fullURL, err)
	}
	if parsed.Scheme != scheme {
		return "", fmt.Errorf("constructed URL scheme changed to %q", parsed.Scheme)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("constructed URL must not contain userinfo")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("constructed URL has no host")
	}
	if parsed.Fragment != "" || strings.Contains(fullURL, "#") {
		return "", fmt.Errorf("constructed URL must not contain a fragment")
	}

	if port != 0 {
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid request.port %d (must be 1-65535)", port)
		}
		if parsed.Port() != "" {
			return "", fmt.Errorf("port set in both request.port (%d) and path %q", port, pathTemplate)
		}
		if !isDefaultPort(scheme, port) {
			parsed.Host = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))
			return parsed.String(), nil
		}
	}

	return fullURL, nil
}

func isDefaultPort(scheme string, port int) bool {
	return (scheme == "http" && port == 80) || (scheme == "https" && port == 443)
}
