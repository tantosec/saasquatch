package config

// AppConfig holds all the configuration for the application.
type AppConfig struct {
	// Input
	Identifier     string `mapstructure:"identifier"`
	IdentifierFile string `mapstructure:"identifier-file"`
	Rules          string `mapstructure:"rules"`

	// Output
	OutputFile string `mapstructure:"output-file"`
	JSONL      bool   `mapstructure:"jsonl"`

	// Tuning & Performance
	Threads      int `mapstructure:"threads"`
	TestThreads  int `mapstructure:"test-threads"`
	Timeout      int `mapstructure:"timeout"`
	LRUCacheSize int `mapstructure:"lru-cache-size"`

	// HTTP Client Configuration
	Proxy            string `mapstructure:"proxy"`
	IgnoreEnvProxies bool   `mapstructure:"ignore-env-proxies"`
	UserAgent        string `mapstructure:"user-agent"`
	RandomAgent      string `mapstructure:"random-agent"`

	// Filtering
	Tags []string `mapstructure:"tags"`

	// Internal or from config file only
	ProxyErrorPageMatcherString []string `mapstructure:"proxy-errorpage-matcher-string"`
	IdentifierPlaceholder       string   `mapstructure:"identifier-placeholder"`
}

var Cfg AppConfig
