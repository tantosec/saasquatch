package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tantosec/saasquatch/internal/client"
	"github.com/tantosec/saasquatch/internal/config"
	"github.com/tantosec/saasquatch/internal/engine"
	"github.com/tantosec/saasquatch/internal/logging"
	"github.com/tantosec/saasquatch/internal/output"
	"github.com/tantosec/saasquatch/internal/rules"
)

var (
	cfgFile                  string
	Cfg                      config.AppConfig
	initConfigCreatedDefault bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "SaaSquatch",
	Short: "A tool to discover and verify an organisation's presence on various SaaS platforms.",
	Long: `SaaSquatch is a CLI tool to discover and verify an organisation's presence on various SaaS platforms using YAML based rules.

By default, the tool runs in 'Test Rules' mode, which validates your rule files.
Provide an identifier (-i) or an identifier file (-I) to run against one or more live targets.`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		if err := viper.Unmarshal(&Cfg); err != nil {
			return err
		}

		logger := logging.GetLogger()

		timeout := time.Duration(Cfg.Timeout) * time.Second
		client.InitClient(timeout, Cfg.Proxy, Cfg.LRUCacheSize, Cfg.IgnoreEnvProxies)
		logger.Debug("Global HTTP client initialized", "timeout", timeout, "proxy", Cfg.Proxy, "cache_size", Cfg.LRUCacheSize)

		stat, _ := os.Stdin.Stat()
		hasPipedInput := (stat.Mode() & os.ModeCharDevice) == 0

		if cmd.Flags().Changed("identifier") || cmd.Flags().Changed("identifier-file") || hasPipedInput {
			viper.Set("test-rules", false)
		} else {
			viper.Set("test-rules", true)
		}

		return nil
	},

	// The main logic for the command.
	Run: func(cmd *cobra.Command, args []string) {
		logger := logging.GetLogger()

		if regenerate, _ := cmd.PersistentFlags().GetBool("regenerate-config"); regenerate {
			var configPath string
			if cfgFile != "" {
				configPath = cfgFile
			} else {
				configPath = filepath.Join(xdg.ConfigHome, "saasquatch", "config.yaml")
			}

			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				logger.Error("Failed to create config directory", "path", filepath.Dir(configPath), "error", err)
				os.Exit(1)
			}

			if _, err := os.Stat(configPath); err == nil && !initConfigCreatedDefault {
				backupPath := configPath + ".bak-" + time.Now().Format("20060102-150405")
				existing, err := os.ReadFile(configPath)
				if err != nil {
					logger.Error("Failed to read existing config for backup", "path", configPath, "error", err)
					os.Exit(1)
				}
				if err := os.WriteFile(backupPath, existing, 0644); err != nil {
					logger.Error("Failed to write config backup", "path", backupPath, "error", err)
					os.Exit(1)
				}
				logger.Info("Backed up existing config", "backup", backupPath)
			}

			if err := os.WriteFile(configPath, defaultYAML, 0644); err != nil {
				logger.Error("Failed to write default config", "path", configPath, "error", err)
				os.Exit(1)
			}
			logger.Info("Wrote fresh default config", "path", configPath)
			return
		}

		logger.Info("Starting SaaSquatch...")
		logger.Debug("Loaded configuration", "config", viper.AllSettings())

		outputManager := output.NewManager(Cfg.JSONL, Cfg.OutputFile)

		// Collect rules
		rulesDir := viper.GetString("rules")
		tags := viper.GetStringSlice("tags")
		allRules, err := rules.LoadAllRulesMatchingTags(rulesDir, tags, Cfg.IdentifierPlaceholder)
		if err != nil {
			logger.Error("SaaSquatch run failed while loading rules", "error", err)
			os.Exit(1)
		}

		if viper.GetBool("test-rules") {
			if !cmd.Flags().Changed("threads") {
				Cfg.Threads = Cfg.TestThreads
			}
			engine.RunTestMode(&Cfg, allRules)
		} else {
			err = engine.RunLiveMode(&Cfg, allRules, outputManager)
		}

		if Cfg.OutputFile != "" {
			logger.Info("Writing JSON report...")
			if err := outputManager.WriteJSONReport(); err != nil {
				logger.Error("Failed to write JSON output file", "path", Cfg.OutputFile, "error", err)
			}
		}

		if err != nil {
			logger.Error("SaaSquatch run failed", "error", err)
			os.Exit(1)
		}

		logger.Info("SaaSquatch run completed successfully.")
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// initConfig reads in config file and ENV variables if set.
// This runs before PersistentPreRunE, so the logger is NOT available here.
func initConfig() {

	logging.InitLogger(viper.GetInt("verbose"))
	logger := logging.GetLogger()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		logger.Debug("Using config file from flag", "path", cfgFile)
	} else {
		configDir := filepath.Join(xdg.ConfigHome, "saasquatch")

		if info, err := os.Stat(configDir); err == nil && !info.IsDir() {
			logger.Error("Config path exists as a file, but it needs to be a directory.", "path", configDir)
			os.Exit(1)
		}

		if err := os.MkdirAll(configDir, 0755); err != nil {
			logger.Error("Failed to create config directory", "path", configDir, "error", err)
			os.Exit(1)
		}

		configFilePath := filepath.Join(configDir, "config.yaml")

		if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
			logger.Info("Creating default config file.", "path", configFilePath)

			if err := os.WriteFile(configFilePath, defaultYAML, 0644); err != nil {
				logger.Error("Failed to write default config file", "error", err)
				os.Exit(1)
			}
			initConfigCreatedDefault = true
		}

		viper.AddConfigPath(configDir)
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.ReadInConfig()
}

func init() {

	// --- INITIALIZE CONFIG ---
	cobra.OnInitialize(initConfig)

	// --- DEFINE ALL FLAGS ---
	// General
	defaultConfigPath := filepath.Join(xdg.ConfigHome, "saasquatch", "config.yaml")
	configHelpText := fmt.Sprintf("config file (default is \"%s\")", defaultConfigPath)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", configHelpText)

	// Input
	rootCmd.PersistentFlags().StringP("identifier", "i", "", "Test a single identifier (infers live mode)")
	rootCmd.PersistentFlags().StringP("identifier-file", "I", "", "Path to a file of identifiers (infers live mode)")
	rootCmd.PersistentFlags().StringP("rules", "r", "./rules/", "Path to a YAML rule file or a directory of rules")

	// Output
	rootCmd.PersistentFlags().StringP("output-file", "o", "", "Path to write JSON output file")
	rootCmd.PersistentFlags().Bool("jsonl", false, "Enable line-delimited JSON output for STDOUT")
	rootCmd.PersistentFlags().CountP("verbose", "v", "Enable debug logging (-v)")

	// Tuning & Performance
	rootCmd.PersistentFlags().IntP("threads", "t", 50, "Number of concurrent threads/goroutines")
	rootCmd.PersistentFlags().Int("timeout", 10, "Request timeout in seconds")
	rootCmd.PersistentFlags().Int("lru-cache-size", 1024, "Maximum number of responses to keep in the LRU cache")

	// HTTP Client Configuration
	rootCmd.PersistentFlags().String("proxy", "", "HTTP/SOCKS5 proxy URL (e.g., http://127.0.0.1:8080)")
	rootCmd.PersistentFlags().Bool("ignore-env-proxies", false, "Ignore HTTP_PROXY/HTTPS_PROXY/ALL_PROXY environment variables (force direct unless --proxy is set)")
	rootCmd.PersistentFlags().String("user-agent", "SaaSquatch", "Set a custom User-Agent string")
	rootCmd.PersistentFlags().String("random-agent", "", "Path to a file of User-Agents for random selection")

	// Filtering
	rootCmd.PersistentFlags().StringSlice("tags", []string{}, "Comma-separated list of tags to run (e.g., \"dev,chat\")")

	// Config management
	rootCmd.PersistentFlags().Bool("regenerate-config", false, "Back up the existing config file and write a fresh default, then exit")

	// --- BIND ALL FLAGS TO VIPER ---
	viper.BindPFlag("identifier", rootCmd.PersistentFlags().Lookup("identifier"))
	viper.BindPFlag("identifier-file", rootCmd.PersistentFlags().Lookup("identifier-file"))
	viper.BindPFlag("rules", rootCmd.PersistentFlags().Lookup("rules"))
	viper.BindPFlag("output-file", rootCmd.PersistentFlags().Lookup("output-file"))
	viper.BindPFlag("jsonl", rootCmd.PersistentFlags().Lookup("jsonl"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("threads", rootCmd.PersistentFlags().Lookup("threads"))
	viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
	viper.BindPFlag("proxy", rootCmd.PersistentFlags().Lookup("proxy"))
	viper.BindPFlag("ignore-env-proxies", rootCmd.PersistentFlags().Lookup("ignore-env-proxies"))
	viper.BindPFlag("user-agent", rootCmd.PersistentFlags().Lookup("user-agent"))
	viper.BindPFlag("random-agent", rootCmd.PersistentFlags().Lookup("random-agent"))
	viper.BindPFlag("tags", rootCmd.PersistentFlags().Lookup("tags"))
	viper.BindPFlag("lru-cache-size", rootCmd.PersistentFlags().Lookup("lru-cache-size"))

	// --- CONFIGURE DEFAULTS ---
	viper.SetDefault("proxy-errorpage-matcher-string", []string{})
	viper.SetDefault("identifier-placeholder", "IDENTIFIER")
	viper.SetDefault("test-threads", 100)
}
