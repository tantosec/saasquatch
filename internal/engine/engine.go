package engine

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/tantosec/saasquatch/internal/config"
	"github.com/tantosec/saasquatch/internal/constants"
	"github.com/tantosec/saasquatch/internal/logging"
	"github.com/tantosec/saasquatch/internal/output"
	"github.com/tantosec/saasquatch/internal/requester"
	"github.com/tantosec/saasquatch/internal/rules"
	"github.com/tantosec/saasquatch/internal/util"
)

type job struct {
	Rule       *rules.Rule
	Identifier string
}

type testResult struct {
	skipped bool
	good    bool
	rule    rules.Rule
	err     error
}

func evalLeaf(leaf rules.Leaf, resp *http.Response, identifier string, identifierPlaceholder string) (bool, error) {
	logger := logging.GetLogger()

	if len(leaf.Status) > 0 {
		logger.Debug("Status Check", "Code", resp.StatusCode, "Valid Codes", leaf.Status)
		return slices.Contains(leaf.Status, resp.StatusCode), nil
	}

	if len(leaf.Body) > 0 {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, constants.MaxBodyReadBytes))
		if err != nil {
			return false, fmt.Errorf("failed to read response body: %w", err)
		}
		resp.Body.Close()                                    // Close the original body
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Replace it so it can be read again
		return anyPatternMatches(leaf.Body, string(bodyBytes), identifier, identifierPlaceholder, leaf.CaseSensitive), nil
	}

	// Header leaf: every named header must match at least one of its patterns.
	for name, patterns := range leaf.Header {
		matched := false
		for _, value := range resp.Header.Values(name) {
			if anyPatternMatches(patterns, value, identifier, identifierPlaceholder, leaf.CaseSensitive) {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func anyPatternMatches(patterns []string, target, identifier, identifierPlaceholder string, caseSensitive bool) bool {
	logger := logging.GetLogger()
	escapedIdentifier := regexp.QuoteMeta(identifier)
	for _, pattern := range patterns {
		pattern = strings.ReplaceAll(pattern, identifierPlaceholder, escapedIdentifier)
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Error("Invalid regex pattern in rule", "pattern", pattern, "error", err)
			continue
		}
		if re.MatchString(target) {
			logger.Debug("Regex matched", "pattern", pattern)
			return true
		}
	}
	return false
}

// test a single rule against a single identifier
func processRule(cfg *config.AppConfig, rule rules.Rule, identifier string) (bool, map[string]any, error) {
	logger := logging.GetLogger()

	reqCfg := requester.RequesterConfig{
		IdentifierPlaceholder: cfg.IdentifierPlaceholder,
		UserAgent:             cfg.UserAgent,
		RandomAgentFile:       cfg.RandomAgent,
		ProxyErrorStrings:     cfg.ProxyErrorPageMatcherString,
		ProxyInUse:            proxyInUse(cfg),
	}

	// Make the request
	resp, err := requester.RequestFromRule(reqCfg, rule.Request, identifier)
	if err != nil {
		return false, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	env := make(map[string]any, len(rule.Detection))
	for name, leaf := range rule.Detection {
		match, err := evalLeaf(leaf, resp, identifier, reqCfg.IdentifierPlaceholder)
		if err != nil {
			return false, nil, fmt.Errorf("failed to evaluate detection %q: %w", name, err)
		}
		env[name] = match
	}

	out, err := expr.Run(rule.Cond, env)
	if err != nil {
		return false, nil, fmt.Errorf("failed to evaluate condition: %w", err)
	}
	found := out.(bool)

	var captures map[string]any
	if found && len(rule.Captures) > 0 {
		captures = extractCaptures(rule.Captures, resp, identifier, reqCfg.IdentifierPlaceholder)
	}

	logger.Debug("Test Rule", "Rule Name", rule.Name, "Detections", env, "Found", found, "Captures", captures)
	return found, captures, nil
}

// extractCaptures runs each capture's regex over the whole serialized response
// (headers + body) and returns the matched values. Only called when the rule
// fired and has captures, so the body read here is never paid otherwise.
func extractCaptures(captures map[string]rules.Capture, resp *http.Response, identifier, identifierPlaceholder string) map[string]any {
	logger := logging.GetLogger()

	var sb strings.Builder
	for name, values := range resp.Header {
		for _, v := range values {
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, constants.MaxBodyReadBytes))
	if err != nil {
		logger.Error("Failed to read response body for captures", "error", err)
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	sb.Write(bodyBytes)
	respText := sb.String()

	escapedIdentifier := regexp.QuoteMeta(identifier)
	results := make(map[string]any, len(captures))
	for name, capture := range captures {
		pattern := strings.ReplaceAll(capture.Regex, identifierPlaceholder, escapedIdentifier)
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Error("Invalid capture regex in rule", "capture", name, "pattern", pattern, "error", err)
			continue
		}
		if capture.All {
			matches := re.FindAllStringSubmatch(respText, -1)
			if len(matches) == 0 {
				continue
			}
			values := make([]string, len(matches))
			for i, m := range matches {
				values[i] = captureValue(m)
			}
			results[name] = values
		} else {
			if m := re.FindStringSubmatch(respText); m != nil {
				results[name] = captureValue(m)
			}
		}
	}
	return results
}

// captureValue returns the first capture group, or the whole match if the regex has no group.
func captureValue(submatch []string) string {
	if len(submatch) > 1 {
		return submatch[1]
	}
	return submatch[0]
}

// proxyInUse reports whether requests are routed through a proxy, either an
// explicit --proxy or an honored proxy environment variable.
func proxyInUse(cfg *config.AppConfig) bool {
	if cfg.Proxy != "" {
		return true
	}
	if cfg.IgnoreEnvProxies {
		return false
	}
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// hostDoesNotExist reports whether err is a DNS "no such host" failure.
func hostDoesNotExist(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// isTimeout reports whether err is a request timeout, covering both an explicit
// context deadline and the http.Client Timeout (which surfaces as a net.Error).
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func testRule(cfg *config.AppConfig, rule rules.Rule) (bool, error) {
	logger := logging.GetLogger()

	// Try the known good identifier and see if it works
	knownGoodIdentifierMatched, _, err := processRule(cfg, rule, rule.KnownGood)
	if err != nil {
		return false, fmt.Errorf("failed to test rule %s from %s. Error: %w", rule.Name, rule.FilePath, err)
	}

	// Try a random identifier (hopefully fails) to ensure the rule works
	randomIdentifierMatched, _, err := processRule(cfg, rule, util.RandomStringWithSeed(24, rule.FilePath))
	if err != nil {
		if hostDoesNotExist(err) {
			// This is the desired outcome for a direct connection to a non-existent host.
			logger.Debug("Request for random identifier failed with expected DNS error.", "rule", rule.Name, "error", err.Error())
			return knownGoodIdentifierMatched, nil

		} else if strings.Contains(err.Error(), "host unreachable") {
			// This is the desired outcome when a SOCKS proxy cannot reach the non-existent host.
			logger.Debug("Request for random identifier failed with expected SOCKS error.", "rule", rule.Name, "error", err.Error())
			return knownGoodIdentifierMatched, nil

		} else if errors.Is(err, requester.ErrProxyErrorPage) {
			// This is the desired outcome when the proxy inserts an error page
			logger.Debug("Request for random identifier failed with proxy error page.", "rule", rule.Name, "error", err.Error())
			return knownGoodIdentifierMatched, nil

		} else {
			// This is an unexpected error (timeout, connection refused, TLS error, etc.).
			return false, fmt.Errorf("failed on random identifier due to unexpected network error: %w", err)
		}
	}

	if knownGoodIdentifierMatched && !randomIdentifierMatched {
		// Rule is working as expected
		logger.Debug("✅ Rule worked as expected, service would be marked [Found]", "Rule", rule.Name, "Identifier", rule.KnownGood, "Rule File", rule.FilePath)
		return true, nil
	} else {
		// Something went wrong with the rule
		logger.Debug("🚫 Rule failed. The known good and random identifiers returned the same result", "Rule", rule.Name, "Identifier", rule.KnownGood, "Rule File", rule.FilePath)
		return false, nil
	}
}

// RunTestMode executes the 'test-rules' logic.
func RunTestMode(cfg *config.AppConfig, rules []rules.Rule) {
	logger := logging.GetLogger()
	logger.Info("Validating existing rules against known identifiers...")

	totalRules := len(rules)

	numWorkers := cfg.Threads
	if numWorkers < 1 {
		logger.Warn("Invalid thread count, defaulting to 1", "configured", cfg.Threads)
		numWorkers = 1
	}
	ruleJobs := make(chan int, numWorkers*2)
	results := make(chan testResult, numWorkers*2)
	var wg sync.WaitGroup

	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range ruleJobs {
				rule := rules[idx]
				ruleWasGood, ruleTestErr := testRule(cfg, rule)
				results <- testResult{
					skipped: ruleTestErr != nil && isTimeout(ruleTestErr),
					good:    ruleWasGood,
					rule:    rule,
					err:     ruleTestErr,
				}
			}
		}()
	}

	go func() {
		for i := range rules {
			ruleJobs <- i
		}
		close(ruleJobs)
		wg.Wait()
		close(results)
	}()

	var goodRules int
	var skippedRules int
	for r := range results {
		if r.skipped {
			skippedRules++
			logger.Warn("[~] Rule validation skipped: request timed out", "Rule", r.rule.Name, "File", r.rule.FilePath)
			continue
		}
		if r.err != nil {
			logger.Error("failed to test rule", "Rule", r.rule.Name, "File", r.rule.FilePath, "Error", r.err)
		}
		if r.good {
			goodRules++
		} else {
			logger.Warn("[-] Rule failed test", "Rule", r.rule.Name, "File", r.rule.FilePath)
		}
	}

	testedRules := totalRules - skippedRules
	var percentage float32
	if testedRules > 0 {
		percentage = float32(goodRules) / float32(testedRules) * 100
	}

	logger.Info("'Test Rules' mode finished.")
	logger.Info("Stats:", "Working rules", goodRules, "Tested Rules", testedRules, "Skipped (timeout)", skippedRules, "Total Rules", totalRules, "Percentage", fmt.Sprintf("%.2f", percentage))
}

// RunLiveMode executes the main 'live' logic against specified identifiers using a pool of concurrent workers.
func RunLiveMode(cfg *config.AppConfig, rules []rules.Rule, outputManager *output.Manager) error {
	logger := logging.GetLogger()

	identifiersToTest, err := getIdentifiers(cfg.IdentifierFile, cfg.Identifier)
	if err != nil {
		return fmt.Errorf("failed to get identifiers: %w", err)
	}
	if len(identifiersToTest) == 0 {
		logger.Error("No identifiers to test. Please provide an identifier with -i or a file with -I.")
		return nil
	}

	if len(rules) == 0 {
		logger.Error("No rules to test.")
		return nil
	}
	totalJobs := len(identifiersToTest) * len(rules)

	logger.Info("Starting live run", "identifier_count", len(identifiersToTest), "rule_count", len(rules), "total_jobs", totalJobs)

	numWorkers := cfg.Threads
	if numWorkers < 1 {
		logger.Warn("Invalid thread count, defaulting to 1", "configured", cfg.Threads)
		numWorkers = 1
	}
	jobs := make(chan job, numWorkers*2)
	var wg sync.WaitGroup

	// --- Start Worker Goroutines ---
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := range jobs {
				fullPath, urlErr := requester.BuildRequestURL(j.Rule.Request.Protocol, j.Rule.Request.Path, j.Rule.Request.Port, cfg.IdentifierPlaceholder, j.Identifier)
				if urlErr != nil {
					fullPath = fmt.Sprintf("%s://%s", j.Rule.Request.Protocol, j.Rule.Request.Path)
				}

				status, captures, err := processRule(cfg, *j.Rule, j.Identifier)

				result := output.Result{
					Timestamp:  time.Now().UTC(),
					Identifier: j.Identifier,
					RuleName:   j.Rule.Name,
					Service:    j.Rule.Service,
					Found:      status,
					URL:        fullPath,
					Captures:   captures,
				}

				if err != nil {
					if hostDoesNotExist(err) {
						// Subdomain does not resolve: the org is simply not present on
						// this service. A definitive negative, not a failed request.
						logger.Debug("Identifier not present on service (host does not resolve)", "rule", j.Rule.Name, "identifier", j.Identifier)
					} else {
						logger.Debug("Request failed unexpectedly during live run", "rule", j.Rule.Name, "identifier", j.Identifier, "error", err)
						result.Error = err.Error()
					}
				}

				outputManager.HandleResult(result)
			}
		}(w)
	}

	// --- Job Production ---
	for _, identifier := range identifiersToTest {
		for i := range rules {
			jobs <- job{Rule: &rules[i], Identifier: identifier}
		}
	}
	close(jobs)

	// Block until all goroutines have called wg.Done().
	wg.Wait()

	summary := outputManager.Summary()
	logger.Info("Scan finished", "total", summary.Total, "found", summary.Found, "errored", summary.Errored)

	if summary.Errored > 0 {
		logger.Warn("Some requests failed; results may be incomplete", "errored", summary.Errored, "total", summary.Total)
	}

	if summary.Total > 0 && summary.Errored == summary.Total {
		return fmt.Errorf("all %d requests failed; no rule could be evaluated (check connectivity and proxy settings)", summary.Total)
	}

	return nil
}

func identifiersFromStdin() ([]string, error) {
	logger := logging.GetLogger()
	logger.Info("No identifier (-i) or identifier file (-I) given; reading identifiers from stdin, one per line (press Ctrl-D when done)")

	identifiers := make([]string, 0)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		identifier := strings.TrimSpace(scanner.Text())
		if identifier != "" {
			identifiers = append(identifiers, identifier)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	logger.Debug("Loaded identifiers from stdin", "count", len(identifiers))
	return identifiers, nil
}

// getIdentifiers retrieves the list of identifiers to test from the configuration
func getIdentifiers(identifierFile string, identifier string) ([]string, error) {

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return identifiersFromStdin()
	}

	if identifierFile != "" {
		return identifiersFromFile(identifierFile)
	}

	if identifier != "" {
		return []string{identifier}, nil
	}

	return []string{}, nil
}

// identifiersFromFile reads a file where each line is an identifier
func identifiersFromFile(filePath string) ([]string, error) {
	logger := logging.GetLogger()
	logger.Debug("Reading identifiers from file", "path", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	identifiers := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		identifier := strings.TrimSpace(scanner.Text())
		if identifier != "" {
			identifiers = append(identifiers, identifier)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	logger.Debug("Loaded identifiers from file", "count", len(identifiers))
	return identifiers, nil
}
