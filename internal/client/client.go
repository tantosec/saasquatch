package client

import (
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/tantosec/saasquatch/internal/constants"
	"github.com/tantosec/saasquatch/internal/httpcache"
	"github.com/tantosec/saasquatch/internal/logging"
	"golang.org/x/net/proxy"
)

var (
	globalClient *http.Client
	once         sync.Once
)

func InitClient(timeout time.Duration, proxyURL string, cacheSize int, ignoreEnvProxies bool) {
	once.Do(func() {
		logger := logging.GetLogger()

		// Create base http.Transport with default settings.
		baseTransport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		if ignoreEnvProxies {
			baseTransport.Proxy = nil
		}

		// Configure the transport with a specific proxy if provided
		if proxyURL != "" {
			parsedProxyURL, err := url.Parse(proxyURL)

			if err != nil || parsedProxyURL.Scheme == "" || parsedProxyURL.Host == "" {
				logger.Fatalf("Invalid proxy URL specified: %s. Must include a scheme and host (e.g., http://localhost:8080)", proxyURL)
			}

			if parsedProxyURL.Port() == "" {
				logger.Fatalf("Proxy URL must include an explicit port: %s (e.g., socks5://127.0.0.1:1080)", proxyURL)
			}

			conn, err := net.DialTimeout("tcp", parsedProxyURL.Host, constants.ProxyDialTimeout)
			if err != nil {
				logger.Fatalf("Could not connect to proxy at %s: %v", parsedProxyURL.Host, err)
			}
			conn.Close()

			switch parsedProxyURL.Scheme {

			case "http", "https":
				baseTransport.Proxy = http.ProxyURL(parsedProxyURL)

			case "socks5", "socks5h":

				forward := &net.Dialer{
					Timeout:   constants.Timeout,
					KeepAlive: constants.KeepAlive,
				}

				dialer, err := proxy.FromURL(parsedProxyURL, forward)
				if err != nil {
					logger.Fatalf("Failed to create SOCKS5 dialer for proxy %s: %v", proxyURL, err)
				}

				contextDialer, ok := dialer.(proxy.ContextDialer)
				if !ok {
					logger.Fatalf("SOCKS5 dialer for proxy %s does not support context-aware dialing", proxyURL)
				}
				baseTransport.DialContext = contextDialer.DialContext

			default:
				logger.Fatalf("Unsupported proxy scheme: %s. Only http, https, and socks5 are supported.", parsedProxyURL.Scheme)
			}
		}

		// Wrap the base transport with the caching layer
		cachingTransport, err := httpcache.NewCachingRoundTripper(cacheSize, baseTransport)
		if err != nil {
			logger.Fatalf("Failed to create caching transport: %v", err)
		}

		// Assemble the final client and assign it to the global instance
		globalClient = &http.Client{
			Transport: cachingTransport,
			Timeout:   timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	})
}

func GetClient() *http.Client {
	if globalClient == nil {
		logging.GetLogger().Fatal("HTTP client requested before initialization; this is a bug (InitClient must be called first)")
	}
	return globalClient
}
