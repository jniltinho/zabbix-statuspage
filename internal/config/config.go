package config

import "fmt"

type Config struct {
	Server              ServerConfig   `mapstructure:"server"`
	Zabbix              ZabbixConfig   `mapstructure:"zabbix"`
	TriggerTags         []Tag          `mapstructure:"trigger_tags"`
	Segments            []Segment      `mapstructure:"segments"`
	ExternalStatuspages []ExternalLink `mapstructure:"external_statuspages"`
}

type ServerConfig struct {
	Addr    string `mapstructure:"addr"`
	Port    int    `mapstructure:"port"`
	TLS     bool   `mapstructure:"tls"`
	TLSCert string `mapstructure:"tls_cert"`
	TLSKey  string `mapstructure:"tls_key"`
}

type ZabbixConfig struct {
	APIURL   string `mapstructure:"api_url"`
	APIToken string `mapstructure:"api_token"`
	CacheTTL int    `mapstructure:"cache_ttl"`
}

type Tag struct {
	Tag   string `mapstructure:"tag"`
	Value string `mapstructure:"value"`
}

type Segment struct {
	Name        string    `mapstructure:"name"`
	Description string    `mapstructure:"description"`
	Services    []Service `mapstructure:"services"`
}

type Service struct {
	ZabbixHost  string `mapstructure:"zabbix_host"`
	DisplayHost string `mapstructure:"display_host"`
	Description string `mapstructure:"description"`
}

type ExternalLink struct {
	Name        string `mapstructure:"name"`
	URL         string `mapstructure:"url"`
	Description string `mapstructure:"description"`
}

// Validate checks required fields after viper.Unmarshal.
func Validate(cfg *Config) error {
	if cfg.Zabbix.APIURL == "" {
		return fmt.Errorf("zabbix.api_url is required")
	}
	if cfg.Zabbix.APIToken == "" {
		return fmt.Errorf("zabbix.api_token is required")
	}
	if cfg.Server.TLS {
		if cfg.Server.TLSCert == "" || cfg.Server.TLSKey == "" {
			return fmt.Errorf("tls_cert and tls_key are required when tls is enabled")
		}
	}
	return nil
}
