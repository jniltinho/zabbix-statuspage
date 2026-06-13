package cmd

import (
	"fmt"
	"io/fs"
	"strings"

	"zabbix-statuspage/internal/config"
	"zabbix-statuspage/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newServeCmd(webFS fs.FS) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the status page server",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, webFS)
		},
	}

	cmd.Flags().String("config", "config.toml", "path to config file")
	cmd.Flags().String("addr", "", "bind address (overrides config)")
	cmd.Flags().Int("port", 0, "HTTP/HTTPS port (overrides config)")
	cmd.Flags().Bool("tls", false, "enable TLS (overrides config)")
	cmd.Flags().String("tls-cert", "", "TLS certificate file")
	cmd.Flags().String("tls-key", "", "TLS private key file")
	cmd.Flags().Bool("debug", false, "enable debug mode")

	return cmd
}

// initConfig runs in PersistentPreRunE: sets up viper (prefix + key replacer +
// AutomaticEnv + defaults), reads the config file, then binds flags so the
// full flag > env > file > default precedence chain is respected.
func initConfig(cmd *cobra.Command) error {
	cfgFile, _ := cmd.Flags().GetString("config")

	viper.SetConfigFile(cfgFile)
	viper.SetConfigType("toml")

	// precedence layer 4 – defaults
	viper.SetDefault("server.addr", "0.0.0.0")
	viper.SetDefault("server.port", 3000)
	viper.SetDefault("server.tls", false)
	viper.SetDefault("zabbix.cache_ttl", 30)

	// precedence layer 3 – env vars
	// Prefixed form:  STATUSPAGE_ZABBIX_API_URL, STATUSPAGE_SERVER_PORT, etc.
	// Unprefixed form: ZABBIX_API_URL, ZABBIX_API_TOKEN (backward compat)
	viper.SetEnvPrefix("STATUSPAGE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	_ = viper.BindEnv("zabbix.api_url", "ZABBIX_API_URL")
	_ = viper.BindEnv("zabbix.api_token", "ZABBIX_API_TOKEN")

	// precedence layer 4/3 – config file
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// precedence layer 2 – CLI flags (only bind when explicitly set so
	// unset flags don't shadow config file values via their zero default)
	flagToKey := map[string]string{
		"addr":     "server.addr",
		"port":     "server.port",
		"tls":      "server.tls",
		"tls-cert": "server.tls_cert",
		"tls-key":  "server.tls_key",
		"debug":    "debug",
	}
	for flag, key := range flagToKey {
		if f := cmd.Flags().Lookup(flag); f != nil && f.Changed {
			_ = viper.BindPFlag(key, f)
		}
	}

	return nil
}

func runServe(cmd *cobra.Command, webFS fs.FS) error {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("config unmarshal: %w", err)
	}
	if err := config.Validate(&cfg); err != nil {
		return fmt.Errorf("config validation error: %w", err)
	}

	debug, _ := cmd.Flags().GetBool("debug")
	return server.New(&cfg, webFS, debug, Version).Start()
}
