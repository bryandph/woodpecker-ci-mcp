package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/denysvitali/woodpecker-ci-mcp/internal/auth"
	"github.com/denysvitali/woodpecker-ci-mcp/internal/client"
	"github.com/denysvitali/woodpecker-ci-mcp/internal/config"
	"github.com/denysvitali/woodpecker-ci-mcp/internal/server"
)

var (
	cfgFile string
	logger  = logrus.New()

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500")).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0EA5E9"))
)

var rootCmd = &cobra.Command{
	Use:   "woodpecker-mcp",
	Short: "MCP Server for Woodpecker CI integration",
	Long: `Woodpecker MCP Server provides AI agents with access to Woodpecker CI
build statuses, pipeline management, and repository information through
the Model Context Protocol (MCP).

This server exposes various tools for:
- Listing and managing pipelines
- Viewing build statuses and logs
- Managing repositories
- Starting/stopping builds
- Approving pending pipelines`,
	Version:       "1.0.0",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	Long:  `Start the MCP server to handle requests from AI agents`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Manage Woodpecker MCP server configuration`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfig()
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration",
	Long:  `Interactive setup of Woodpecker MCP server configuration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initConfig()
	},
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication management",
	Long:  `Manage authentication tokens for Woodpecker CI`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Woodpecker CI",
	Long:  `Interactively authenticate with your Woodpecker CI server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return loginAuth()
	},
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test connection to Woodpecker CI",
	Long:  `Test the connection to your configured Woodpecker CI server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return testConnection()
	},
}

var testToolsCmd = &cobra.Command{
	Use:   "test-tools",
	Short: "Test all MCP tools",
	Long:  `Test all available MCP tools with interactive prompts or predefined test cases`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return testMCPTools()
	},
}

func init() {
	cobra.OnInitialize(initializeConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/woodpecker-mcp/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "text", "log format (text, json)")

	// Bind flags to viper
	if err := viper.BindPFlag("logging.level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		logger.WithError(err).Fatal("Failed to bind log-level flag")
	}
	if err := viper.BindPFlag("logging.format", rootCmd.PersistentFlags().Lookup("log-format")); err != nil {
		logger.WithError(err).Fatal("Failed to bind log-format flag")
	}

	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(testToolsCmd)

	// Config subcommands
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)

	// Auth subcommands
	authCmd.AddCommand(authLoginCmd)

	// Set default command to serve
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "serve")
	}
}

func initializeConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search for config in home directory
		configDir := filepath.Join(home, ".config", "woodpecker-mcp")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Environment variables
	viper.SetEnvPrefix("WOODPECKER_MCP")
	viper.AutomaticEnv()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		// It's okay if config file doesn't exist
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			logger.WithError(err).Debug("Error reading config file")
		}
	}

	// Configure logger
	configureLogger()
}

func configureLogger() {
	// Set log level
	level := viper.GetString("logging.level")
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Set log format
	format := viper.GetString("logging.format")
	if format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}
}

func runServer() error {
	fmt.Fprintln(os.Stderr, titleStyle.Render("Starting Woodpecker MCP Server"))

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Configuration error: %v", err)))
		fmt.Fprintln(os.Stderr, infoStyle.Render("Run 'woodpecker-mcp config init' to set up configuration"))
		return err
	}

	// Create Woodpecker client
	wclient, err := client.New(client.Config{
		URL:   cfg.Woodpecker.URL,
		Token: cfg.Woodpecker.Token,
	}, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Failed to connect to Woodpecker: %v", err)))
		return err
	}

	// Create MCP server
	mcpServer, err := server.NewMCPServer(cfg, wclient, logger)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	fmt.Fprintln(os.Stderr, successStyle.Render("MCP server started successfully"))
	fmt.Fprintln(os.Stderr, infoStyle.Render(fmt.Sprintf("Connected to Woodpecker server: %s", cfg.Woodpecker.URL)))

	// Start serving
	return mcpServer.Serve()
}

func showConfig() error {
	fmt.Println(titleStyle.Render("Current Configuration"))

	cfg, err := config.Load()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to load configuration: %v", err)))
		return err
	}

	fmt.Printf("Server Name: %s\n", infoStyle.Render(cfg.Server.Name))
	fmt.Printf("Server Version: %s\n", infoStyle.Render(cfg.Server.Version))
	fmt.Printf("Woodpecker URL: %s\n", infoStyle.Render(cfg.Woodpecker.URL))

	if cfg.Woodpecker.Token != "" {
		fmt.Printf("Woodpecker Token: %s\n", infoStyle.Render(auth.MaskToken(cfg.Woodpecker.Token)))
	} else {
		fmt.Printf("Woodpecker Token: %s\n", errorStyle.Render("Not configured"))
	}

	fmt.Printf("Log Level: %s\n", infoStyle.Render(cfg.Logging.Level))
	fmt.Printf("Log Format: %s\n", infoStyle.Render(cfg.Logging.Format))

	return nil
}

func initConfig() error {
	fmt.Println(titleStyle.Render("Woodpecker MCP Configuration Setup"))

	// Ensure config directory exists
	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Get Woodpecker URL
	url, err := auth.PromptForURL()
	if err != nil {
		return err
	}

	// Get authentication token
	token, err := auth.PromptForToken(url)
	if err != nil {
		return err
	}

	// Confirm configuration
	if !auth.ConfirmConfiguration(url, auth.MaskToken(token)) {
		fmt.Println(errorStyle.Render("Configuration cancelled"))
		return nil
	}

	// Create config file
	configDir, err := config.GetConfigDir()
	if err != nil {
		return err
	}

	configFile := filepath.Join(configDir, "config.yaml")

	configContent := fmt.Sprintf(`# Woodpecker MCP Server Configuration
server:
  name: "woodpecker-mcp"
  version: "1.0.0"

woodpecker:
  url: "%s"
  token: "%s"

logging:
  level: "info"
  format: "text"
`, url, token)

	err = os.WriteFile(configFile, []byte(configContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Println(successStyle.Render(fmt.Sprintf("Configuration saved to: %s", configFile)))
	fmt.Println(infoStyle.Render("You can now run 'woodpecker-mcp serve' to start the server"))

	return nil
}

func loginAuth() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.Woodpecker.URL == "" {
		fmt.Println(errorStyle.Render("Woodpecker URL not configured"))
		fmt.Println(infoStyle.Render("Run 'woodpecker-mcp config init' to set up configuration"))
		return nil
	}

	// Get new token
	token, err := auth.PromptForToken(cfg.Woodpecker.URL)
	if err != nil {
		return err
	}

	// Test the token
	wclient, err := client.New(client.Config{
		URL:   cfg.Woodpecker.URL,
		Token: token,
	}, logger)
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Authentication failed: %v", err)))
		return err
	}

	// Get user info to verify token
	user, err := wclient.GetCurrentUser()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to verify authentication: %v", err)))
		return err
	}

	fmt.Println(successStyle.Render(fmt.Sprintf("Successfully authenticated as: %s", user.Login)))

	// Update config file with new token
	viper.Set("woodpecker.token", token)

	// Write updated config
	configDir, err := config.GetConfigDir()
	if err != nil {
		return err
	}
	configFile := filepath.Join(configDir, "config.yaml")

	err = viper.WriteConfigAs(configFile)
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to save token: %v", err)))
		return err
	}

	fmt.Println(successStyle.Render("Authentication token updated successfully"))

	return nil
}

func testConnection() error {
	fmt.Println(titleStyle.Render("Testing Woodpecker Connection"))

	cfg, err := config.Load()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to load configuration: %v", err)))
		return err
	}

	if err := cfg.Validate(); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Configuration error: %v", err)))
		return err
	}

	// Test connection
	wclient, err := client.New(client.Config{
		URL:   cfg.Woodpecker.URL,
		Token: cfg.Woodpecker.Token,
	}, logger)
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Connection failed: %v", err)))
		return err
	}

	// Get user info
	user, err := wclient.GetCurrentUser()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to get user info: %v", err)))
		return err
	}

	// Get repositories count
	repos, err := wclient.ListRepositories()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to list repositories: %v", err)))
		return err
	}

	fmt.Println(successStyle.Render("Connection successful!"))
	fmt.Printf("Server URL: %s\n", infoStyle.Render(cfg.Woodpecker.URL))
	fmt.Printf("Authenticated as: %s\n", infoStyle.Render(user.Login))
	fmt.Printf("Available repositories: %s\n", infoStyle.Render(fmt.Sprintf("%d", len(repos))))

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
