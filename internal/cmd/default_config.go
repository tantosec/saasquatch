package cmd

var (
	defaultYAML = []byte(`
# Inputs
rules: ./rules/ # Path to a directory of YAML rules

# Output
# output-file: ./saasquatch.json # Path to write output file
# verbose: 0 # Verbosity levels (-v) 0 or 1

# Tuning & Performance
threads: 50 # Number of concurrent threads/goroutines
test-threads: 100 # Concurrent threads used in 'Test Rules' mode
timeout: 10 # Request timeout in seconds

# Client Configuration
# proxy: http://127.0.0.1:8080 # HTTP/SOCKS5 proxy URL (e.g., http://127.0.0.1:8080)
# A list of strings that, if found in a response body, indicate a proxy error page. Useful for filtering out captive portal or proxy block pages.
proxy-errorpage-matcher-string:
  - <h1>Burp Suite Professional</h1>
  - <title>Burp Suite Community Edition</title>
  - <title>Burp Suite</title>
user-agent: SaaSquatch # Set a custom User-Agent string
# random-agent: # File containing user agents

# Advanced — the placeholder token substituted into rule paths; change only if you know what you're doing
identifier-placeholder: IDENTIFIER
lru-cache-size: 1024
`)
)
