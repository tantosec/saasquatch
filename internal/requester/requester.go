package requester

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tantosec/saasquatch/internal/client"
	"github.com/tantosec/saasquatch/internal/constants"
	"github.com/tantosec/saasquatch/internal/logging"
	"github.com/tantosec/saasquatch/internal/rules"
	"github.com/tantosec/saasquatch/internal/util"
)

var ErrProxyErrorPage = errors.New("response matches a configured proxy error page")

type RequesterConfig struct {
	IdentifierPlaceholder string
	UserAgent             string
	RandomAgentFile       string
	ProxyErrorStrings     []string
	ProxyInUse            bool
}

func RequestFromRule(reqCfg RequesterConfig, ruleRequest rules.Request, identifier string) (*http.Response, error) {
	logger := logging.GetLogger()

	fullURL, err := BuildRequestURL(ruleRequest.Protocol, ruleRequest.Path, ruleRequest.Port, reqCfg.IdentifierPlaceholder, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to build request URL: %w", err)
	}

	logger.Debug("🚀 Making request", "method", ruleRequest.Method, "url", fullURL)

	httpClient := client.GetClient()

	req, err := http.NewRequest(ruleRequest.Method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request for %q: %w", fullURL, err)
	}

	userAgent := reqCfg.UserAgent
	randomAgentFile := reqCfg.RandomAgentFile

	if randomAgentFile != "" && strings.ToLower(userAgent) == "saasquatch" {
		ua, err := util.RandomLineFromFile(randomAgentFile)
		if err != nil {
			logger.Warn("Failed to get random user agent, using default", "file", randomAgentFile, "error", err)
			req.Header.Set("User-Agent", userAgent)
		} else {
			req.Header.Set("User-Agent", ua)
		}
	} else {
		req.Header.Set("User-Agent", userAgent)
	}

	// Execute the request using the global client
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to %q: %w", fullURL, err)
	}

	proxyErrorStrings := reqCfg.ProxyErrorStrings

	// The proxy-error-page scan only makes sense when traffic is actually routed
	// through a proxy; otherwise it reads every response body for nothing.
	if len(proxyErrorStrings) > 0 && reqCfg.ProxyInUse {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, constants.MaxBodyReadBytes))
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to read response body for proxy check: %w", err)
		}
		resp.Body.Close()

		bodyString := string(bodyBytes)

		// Check if any of the configured error strings are in the body.
		for _, errorString := range proxyErrorStrings {
			if strings.Contains(bodyString, errorString) {
				return nil, fmt.Errorf("%w (found: %q)", ErrProxyErrorPage, errorString)
			}
		}

		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resp, nil
}
