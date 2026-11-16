package main

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	APIKey        string   `mapstructure:"api_key"`
	Records       []Record `mapstructure:"dns_records"`
	CheckInterval int      `mapstructure:"check_interval"`
}

type Record struct {
	Domain  string   `mapstructure:"domain"`
	ZoneID  string   `mapstructure:"zone_id"`
	Proxied bool     `mapstructure:"proxied"`
	IPs     []string `mapstructure:"ips"`
}

func parseConfig() (Config, error) {
	var configPath string
	pflag.StringVarP(&configPath, "config", "c", "", "Path to config file")
	pflag.Parse()

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
	}

	var cfg Config

	err := viper.ReadInConfig()
	if err != nil {
		return cfg, fmt.Errorf("Error reading config: %v", err)
	}

	err = viper.Unmarshal(&cfg)
	if err != nil {
		return cfg, fmt.Errorf("Unable to decode config into struct, %v", err)
	}

	if cfg.CheckInterval <= 0 {
		return cfg, fmt.Errorf("check_interval must be a positive number of seconds")
	}

	return cfg, nil
}
