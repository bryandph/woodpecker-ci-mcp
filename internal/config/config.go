package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Woodpecker WoodpeckerConfig `mapstructure:"woodpecker"`
	Server     ServerConfig     `mapstructure:"server"`
	Logging    LoggingConfig    `mapstructure:"logging"`
}

type WoodpeckerConfig struct {
	URL   string `mapstructure:"url"`
	Token string `mapstructure:"token"`
}

type ServerConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// userHomeDirFunc is a variable that allows mocking os.UserHomeDir in tests
var userHomeDirFunc = os.UserHomeDir

func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configuration file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.config/woodpecker-mcp")
	v.AddConfigPath("/etc/woodpecker-mcp")

	// Environment variables
	v.SetEnvPrefix("WOODPECKER_MCP")
	// Nested keys are not visible to Unmarshal through AutomaticEnv alone.
	for key, env := range map[string]string{
		"woodpecker.url":   "WOODPECKER_MCP_WOODPECKER_URL",
		"woodpecker.token": "WOODPECKER_MCP_WOODPECKER_TOKEN",
	} {
		if err := v.BindEnv(key, env); err != nil {
			return nil, fmt.Errorf("failed to bind %s: %w", env, err)
		}
	}
	v.AutomaticEnv()

	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.name", "woodpecker-mcp")
	v.SetDefault("server.version", "1.0.0")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
}

func (c *Config) Validate() error {
	if c.Woodpecker.URL == "" {
		return fmt.Errorf("woodpecker URL is required")
	}
	if c.Woodpecker.Token == "" {
		return fmt.Errorf("woodpecker token is required")
	}
	return nil
}

func GetConfigDir() (string, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, ".config", "woodpecker-mcp")
	return configDir, nil
}

func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(configDir, 0755)
}
