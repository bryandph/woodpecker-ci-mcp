package server

import (
	"fmt"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/denysvitali/woodpecker-ci-mcp/internal/client"
	"github.com/denysvitali/woodpecker-ci-mcp/internal/config"
	"github.com/denysvitali/woodpecker-ci-mcp/tools"
)

type MCPServer struct {
	server *server.MCPServer
	client *client.Client
	logger *logrus.Logger
	config *config.Config
}

func NewMCPServer(cfg *config.Config, wclient *client.Client, logger *logrus.Logger) (*MCPServer, error) {
	// Create MCP server
	mcpServer := server.NewMCPServer(
		cfg.Server.Name,
		cfg.Server.Version,
	)

	mcpSrv := &MCPServer{
		server: mcpServer,
		client: wclient,
		logger: logger,
		config: cfg,
	}

	// Register tools
	if err := mcpSrv.registerTools(); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}

	logger.Info("MCP server initialized with Woodpecker CI tools")
	return mcpSrv, nil
}

func (s *MCPServer) registerTools() error {
	// Create tool manager
	toolManager := tools.NewToolManager(s.client, s.logger)

	// Register tool handlers
	serverTools := toolManager.GetServerTools()
	s.server.AddTools(serverTools...)
	s.logger.WithField("tool_count", len(serverTools)).Info("Registered MCP tools")
	return nil
}

func (s *MCPServer) Serve() error {
	s.logger.Info("Starting MCP server...")
	return server.ServeStdio(s.server)
}

func (s *MCPServer) GetToolsInfo() []ToolInfo {
	return []ToolInfo{
		{
			Name:        "list_pipelines",
			Description: "List pipelines for a repository",
			Category:    "Pipeline Management",
		},
		{
			Name:        "get_pipeline_status",
			Description: "Get the status of a specific pipeline",
			Category:    "Pipeline Management",
		},
		{
			Name:        "start_pipeline",
			Description: "Start (restart) a specific pipeline",
			Category:    "Pipeline Management",
		},
		{
			Name:        "stop_pipeline",
			Description: "Stop a running pipeline",
			Category:    "Pipeline Management",
		},
		{
			Name:        "approve_pipeline",
			Description: "Approve a pending pipeline",
			Category:    "Pipeline Management",
		},
		{
			Name:        "wait_for_pipeline",
			Description: "Wait for a pipeline to finish and summarize the result",
			Category:    "Pipeline Management",
		},
		{
			Name:        "list_repositories",
			Description: "List all repositories accessible to the authenticated user",
			Category:    "Repository Management",
		},
		{
			Name:        "get_repository",
			Description: "Get detailed information about a specific repository",
			Category:    "Repository Management",
		},
		{
			Name:        "get_logs",
			Description: "Get logs for a specific pipeline step",
			Category:    "Log Management",
		},
	}
}

type ToolInfo struct {
	Name        string
	Description string
	Category    string
}
