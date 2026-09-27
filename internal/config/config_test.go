package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create a temporary directory and config file
	tmpDir := t.TempDir()
	configContent := `
woodpecker:
  url: "https://woodpecker.example.com"
  token: "test-token"
server:
  name: "test-server"
  version: "1.0.0"
logging:
  level: "debug"
  format: "json"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Change to temp directory to load config
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalDir) }()
	_ = os.Chdir(tmpDir)

	// Load the config
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify the values
	require.Equal(t, "https://woodpecker.example.com", cfg.Woodpecker.URL)
	require.Equal(t, "test-token", cfg.Woodpecker.Token)
	require.Equal(t, "test-server", cfg.Server.Name)
	require.Equal(t, "1.0.0", cfg.Server.Version)
	require.Equal(t, "debug", cfg.Logging.Level)
	require.Equal(t, "json", cfg.Logging.Format)
}

func TestLoad_MissingConfig(t *testing.T) {
	// Create a temp directory without a config file
	tmpDir := t.TempDir()

	// Change to temp directory (no config file exists)
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalDir) }()
	_ = os.Chdir(tmpDir)

	// Load should succeed with defaults when no config file exists
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should have default values
	require.Equal(t, "woodpecker-mcp", cfg.Server.Name)
	require.Equal(t, "1.0.0", cfg.Server.Version)
	require.Equal(t, "info", cfg.Logging.Level)
	require.Equal(t, "text", cfg.Logging.Format)
}

func TestLoad_EnvironmentCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalDir) }()
	_ = os.Chdir(tmpDir)

	t.Setenv("WOODPECKER_MCP_WOODPECKER_URL", "https://woodpecker.example.com")
	t.Setenv("WOODPECKER_MCP_WOODPECKER_TOKEN", "env-token")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://woodpecker.example.com", cfg.Woodpecker.URL)
	require.Equal(t, "env-token", cfg.Woodpecker.Token)
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Woodpecker: WoodpeckerConfig{
			URL:   "https://woodpecker.example.com",
			Token: "test-token",
		},
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestValidate_MissingURL(t *testing.T) {
	cfg := &Config{
		Woodpecker: WoodpeckerConfig{
			URL:   "",
			Token: "test-token",
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "URL is required")
}

func TestValidate_MissingToken(t *testing.T) {
	cfg := &Config{
		Woodpecker: WoodpeckerConfig{
			URL:   "https://woodpecker.example.com",
			Token: "",
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "token is required")
}

func TestValidate_MissingURLAndToken(t *testing.T) {
	cfg := &Config{
		Woodpecker: WoodpeckerConfig{
			URL:   "",
			Token: "",
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	// Should fail on URL first
	require.Contains(t, err.Error(), "URL is required")
}

func TestGetConfigDir(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expectedDir := filepath.Join(homeDir, ".config", "woodpecker-mcp")

	configDir, err := GetConfigDir()
	require.NoError(t, err)
	require.Equal(t, expectedDir, configDir)
}

func TestGetConfigDir_HomeDirError(t *testing.T) {
	// Save original function and restore after test
	originalUserHomeDir := userHomeDirFunc
	userHomeDirFunc = func() (string, error) {
		return "", os.ErrPermission
	}
	defer func() { userHomeDirFunc = originalUserHomeDir }()

	configDir, err := GetConfigDir()
	require.Error(t, err)
	require.Equal(t, "", configDir)
}

func TestEnsureConfigDir_Success(t *testing.T) {
	// Create a unique temp directory for testing
	tmpDir := t.TempDir()

	// Save original function and restore after test
	originalUserHomeDir := userHomeDirFunc
	userHomeDirFunc = func() (string, error) {
		return tmpDir, nil
	}
	defer func() { userHomeDirFunc = originalUserHomeDir }()

	// Calculate expected config directory path
	expectedConfigDir := filepath.Join(tmpDir, ".config", "woodpecker-mcp")

	// Ensure the directory
	err := EnsureConfigDir()
	require.NoError(t, err)

	// Verify the directory was created
	info, err := os.Stat(expectedConfigDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Calling again should succeed (idempotent)
	err = EnsureConfigDir()
	require.NoError(t, err)
}

func TestEnsureConfigDir_GetConfigDirError(t *testing.T) {
	// Mock GetConfigDir to return an error
	originalUserHomeDir := userHomeDirFunc
	userHomeDirFunc = func() (string, error) {
		return "", os.ErrPermission
	}
	defer func() { userHomeDirFunc = originalUserHomeDir }()

	err := EnsureConfigDir()
	require.Error(t, err)
	require.Equal(t, os.ErrPermission, err)
}
