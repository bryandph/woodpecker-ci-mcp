package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline/errors"
	yaml "go.woodpecker-ci.org/woodpecker/v3/pipeline/frontend/yaml"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline/frontend/yaml/linter"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"

	"github.com/denysvitali/woodpecker-ci-mcp/internal/client"
)

type ToolManager struct {
	client *client.Client
	logger *logrus.Logger
	tools  []mcp.Tool
}

func NewToolManager(wclient *client.Client, logger *logrus.Logger) *ToolManager {
	tm := &ToolManager{
		client: wclient,
		logger: logger,
	}

	tm.initializeTools()
	return tm
}

func (tm *ToolManager) initializeTools() {
	tm.tools = []mcp.Tool{
		{
			Name:        "list_repositories",
			Description: "List all repositories accessible to the authenticated user",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "Include all repositories (default: false, only active repositories)",
					},
				},
			},
		},
		{
			Name:        "get_repository",
			Description: "Get detailed information about a specific repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
				},
			},
		},
		{
			Name:        "list_pipelines",
			Description: "List pipelines for a repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"description": "Maximum number of pipelines to return (default: 10, max: 100)",
					},
				},
			},
		},
		{
			Name:        "get_pipeline_status",
			Description: "Get the status of a specific pipeline",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number (required if not using 'latest')",
					},
					"latest": map[string]interface{}{
						"type":        "boolean",
						"description": "Get the latest pipeline (default: false)",
					},
				},
			},
		},
		{
			Name:        "start_pipeline",
			Description: "Start (restart) a specific pipeline",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number to restart",
					},
					"fork": map[string]interface{}{
						"type":        "boolean",
						"description": "Fork the pipeline (default: false)",
					},
				},
			},
		},
		{
			Name:        "stop_pipeline",
			Description: "Stop a running pipeline",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number to stop",
					},
				},
			},
		},
		{
			Name:        "approve_pipeline",
			Description: "Approve a pending pipeline",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number to approve",
					},
				},
			},
		},
		{
			Name:        "trigger_pipeline",
			Description: "Trigger a new pipeline for a repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"branch": map[string]interface{}{
						"type":        "string",
						"description": "Branch to trigger pipeline for (default: main branch)",
					},
				},
			},
		},
		{
			Name:        "get_logs",
			Description: "Get logs for a specific pipeline step",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number",
					},
					"step_id": map[string]interface{}{
						"type":        "number",
						"description": "Step ID to get logs for",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"description": "Output format: 'json' for structured data or 'text' for plain text (default: json)",
					},
					"lines": map[string]interface{}{
						"type":        "number",
						"description": "Number of lines to return (default: all)",
					},
					"tail": map[string]interface{}{
						"type":        "boolean",
						"description": "Return last N lines instead of first N (default: false for head)",
					},
				},
			},
		},
		{
			Name: "wait_for_pipeline",
			Description: "Block until a pipeline finishes, then return one compact summary. " +
				"Use this instead of repeatedly polling get_pipeline_status. " +
				"On failure, includes the tail of each failed step's logs; on success, no logs. " +
				"Returns early with outcome 'blocked' when the pipeline awaits approval, and " +
				"returns outcome 'timeout' (not an error) if the timeout elapses first.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "number",
						"description": "Repository ID (optional, can use repo_name or infer from git remote)",
					},
					"repo_name": map[string]interface{}{
						"type":        "string",
						"description": "Repository full name (optional, owner/repo, can use repo_id or infer from git remote)",
					},
					"pipeline_number": map[string]interface{}{
						"type":        "number",
						"description": "Pipeline number to wait for (exactly one of pipeline_number, latest, or commit)",
					},
					"latest": map[string]interface{}{
						"type":        "boolean",
						"description": "Wait for the repository's latest pipeline at call time",
					},
					"commit": map[string]interface{}{
						"type":        "string",
						"description": "Commit SHA or prefix; waits for its pipeline to appear, then for it to finish (useful right after a push)",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "number",
						"description": "Maximum time to wait (default: 7200, max: 21600)",
					},
					"poll_interval_seconds": map[string]interface{}{
						"type":        "number",
						"description": "Initial server-side poll interval; backs off to 60 (default: 15, min: 5)",
					},
					"log_lines": map[string]interface{}{
						"type":        "number",
						"description": "Log lines to return from the end of each failed step, 0 for none (default: 100, max: 1000)",
					},
					"wait_on_blocked": map[string]interface{}{
						"type":        "boolean",
						"description": "Keep waiting while the pipeline awaits approval instead of returning (default: false)",
					},
				},
			},
		},
		{
			Name:        "lint_config",
			Description: "Lint a Woodpecker CI pipeline configuration file (local YAML file)",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the pipeline configuration file (.yaml or .yml)",
					},
					"strict": map[string]interface{}{
						"type":        "boolean",
						"description": "Treat warnings as errors (default: false)",
					},
				},
				Required: []string{"path"},
			},
		},
	}
}

func (tm *ToolManager) GetServerTools() []server.ServerTool {
	var serverTools []server.ServerTool
	for _, tool := range tm.tools {
		serverTools = append(serverTools, server.ServerTool{
			Tool:    tool,
			Handler: tm.getToolHandler(tool.Name),
		})
	}
	return serverTools
}

func (tm *ToolManager) getToolHandler(name string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tm.logger.WithFields(logrus.Fields{
			"tool":      name,
			"arguments": request.Params.Arguments,
		}).Debug("Calling tool")

		// Type assert arguments to map[string]interface{}
		arguments, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Invalid arguments format",
					},
				},
				IsError: true,
			}, nil
		}

		switch name {
		case "list_repositories":
			return tm.handleListRepositories(ctx, arguments)
		case "get_repository":
			return tm.handleGetRepository(ctx, arguments)
		case "list_pipelines":
			return tm.handleListPipelines(ctx, arguments)
		case "get_pipeline_status":
			return tm.handleGetPipelineStatus(ctx, arguments)
		case "start_pipeline":
			return tm.handleStartPipeline(ctx, arguments)
		case "stop_pipeline":
			return tm.handleStopPipeline(ctx, arguments)
		case "approve_pipeline":
			return tm.handleApprovePipeline(ctx, arguments)
		case "trigger_pipeline":
			return tm.handleTriggerPipeline(ctx, arguments)
		case "get_logs":
			return tm.handleGetLogs(ctx, arguments)
		case "wait_for_pipeline":
			return tm.handleWaitForPipeline(ctx, request, arguments)
		case "lint_config":
			return tm.handleLintConfig(ctx, arguments)
		default:
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Unknown tool: %s", name),
					},
				},
				IsError: true,
			}, nil
		}
	}
}

func (tm *ToolManager) handleListRepositories(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repositories, err := tm.client.ListRepositories()
	if err != nil {
		tm.logger.WithError(err).Error("Failed to list repositories")
		return tm.errorResult(fmt.Sprintf("Failed to list repositories: %v", err)), nil
	}

	showAll := getBool(arguments, "all", false)
	if !showAll {
		var activeRepos []*woodpecker.Repo
		for _, repo := range repositories {
			if repo.IsActive {
				activeRepos = append(activeRepos, repo)
			}
		}
		repositories = activeRepos
	}

	response := map[string]interface{}{
		"repositories": repositories,
		"total_count":  len(repositories),
		"showing_all":  showAll,
	}

	return tm.jsonResult(response)
}

func (tm *ToolManager) handleGetRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	repo, err := tm.client.GetRepository(repoID)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to get repository: %v", err)), nil
	}

	return tm.jsonResult(repo)
}

func (tm *ToolManager) handleListPipelines(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipelines, err := tm.client.ListPipelines(repoID)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to list pipelines: %v", err)), nil
	}

	// Apply limit with default of 10 and max of 100
	limit := getNumber(arguments, "limit", 10)
	if limit > 100 {
		limit = 100
	}
	if limit < 1 {
		limit = 10
	}

	totalCount := len(pipelines)
	limited := totalCount > int(limit)

	if limited {
		pipelines = pipelines[:int(limit)]
	}

	response := map[string]interface{}{
		"repo_id":     repoID,
		"pipelines":   pipelines,
		"total_count": totalCount,
		"returned":    len(pipelines),
		"limited":     limited,
	}

	return tm.jsonResult(response)
}

func (tm *ToolManager) handleGetPipelineStatus(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	var pipeline interface{}
	latest := getBool(arguments, "latest", false)

	if latest {
		pipeline, err = tm.client.GetLastPipeline(repoID)
	} else {
		pipelineNum, numErr := requireNumber(arguments, "pipeline_number")
		if numErr != nil {
			return tm.errorResult("Either pipeline_number or latest=true must be provided"), nil
		}
		pipeline, err = tm.client.GetPipeline(repoID, int64(pipelineNum))
	}

	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to get pipeline: %v", err)), nil
	}

	return tm.jsonResult(pipeline)
}

func (tm *ToolManager) handleStartPipeline(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipelineNum, err := requireNumber(arguments, "pipeline_number")
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	params := make(map[string]string)
	if getBool(arguments, "fork", false) {
		params["fork"] = "true"
	}

	pipeline, err := tm.client.StartPipeline(repoID, int64(pipelineNum), params)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to start pipeline: %v", err)), nil
	}

	return tm.jsonResult(pipeline)
}

func (tm *ToolManager) handleStopPipeline(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipelineNum, err := requireNumber(arguments, "pipeline_number")
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	err = tm.client.StopPipeline(repoID, int64(pipelineNum))
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to stop pipeline: %v", err)), nil
	}

	response := map[string]interface{}{
		"success":         true,
		"message":         "Pipeline stopped successfully",
		"repo_id":         repoID,
		"pipeline_number": int64(pipelineNum),
	}

	return tm.jsonResult(response)
}

func (tm *ToolManager) handleApprovePipeline(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipelineNum, err := requireNumber(arguments, "pipeline_number")
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipeline, err := tm.client.ApprovePipeline(repoID, int64(pipelineNum))
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to approve pipeline: %v", err)), nil
	}

	return tm.jsonResult(pipeline)
}

func (tm *ToolManager) handleTriggerPipeline(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	// Build pipeline options
	options := &woodpecker.PipelineOptions{
		Branch: getString(arguments, "branch", "main"),
	}

	pipeline, err := tm.client.CreatePipeline(repoID, options)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to trigger pipeline: %v", err)), nil
	}

	return tm.jsonResult(pipeline)
}

func (tm *ToolManager) handleGetLogs(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	pipelineNum, err := requireNumber(arguments, "pipeline_number")
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	stepID, err := requireNumber(arguments, "step_id")
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	format := getString(arguments, "format", "json")
	lines := getNumber(arguments, "lines", 0) // 0 means all lines
	useTail := getBool(arguments, "tail", false)

	logs, err := tm.client.GetStepLogs(repoID, int64(pipelineNum), int64(stepID))
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to get logs: %v", err)), nil
	}

	totalCount := len(logs)
	limited := false

	// Apply line limiting if requested
	if lines > 0 && int(lines) < len(logs) {
		limited = true
		if useTail {
			// Take last N lines
			logs = logs[len(logs)-int(lines):]
		} else {
			// Take first N lines
			logs = logs[:int(lines)]
		}
	}

	if format == "text" {
		var logLines []string
		for _, logEntry := range logs {
			if len(logEntry.Data) > 0 {
				// Data is already decoded as []byte from the API's JSON array of integers
				logLines = append(logLines, string(logEntry.Data))
			}
		}

		var plainText string
		if limited {
			direction := "first"
			if useTail {
				direction = "last"
			}
			plainText = fmt.Sprintf("Logs for repo %d, pipeline %d, step %d (%s %d of %d lines):\n%s",
				repoID, int64(pipelineNum), int64(stepID), direction, len(logs), totalCount, strings.Join(logLines, "\n"))
		} else {
			plainText = fmt.Sprintf("Logs for repo %d, pipeline %d, step %d:\n%s",
				repoID, int64(pipelineNum), int64(stepID), strings.Join(logLines, "\n"))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: plainText,
				},
			},
		}, nil
	}

	// Prepare logs for JSON response (data is already decoded as []byte from the API)
	var decodedLogs []map[string]interface{}
	for _, logEntry := range logs {
		decodedLogs = append(decodedLogs, map[string]interface{}{
			"line": int(logEntry.Line),
			"data": string(logEntry.Data),
		})
	}

	response := map[string]interface{}{
		"repo_id":         repoID,
		"pipeline_number": int64(pipelineNum),
		"step_id":         int64(stepID),
		"logs":            decodedLogs,
		"total_count":     totalCount,
		"returned":        len(logs),
		"limited":         limited,
	}

	if limited {
		response["limit_mode"] = map[string]interface{}{
			"lines": int(lines),
			"tail":  useTail,
		}
	}

	return tm.jsonResult(response)
}

func (tm *ToolManager) jsonResult(data interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func (tm *ToolManager) errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: message,
			},
		},
		IsError: true,
	}
}

func (tm *ToolManager) handleLintConfig(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	filePath := getString(arguments, "path", "")
	if filePath == "" {
		return tm.errorResult("path is required"), nil
	}

	// Validate file extension
	if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
		return tm.errorResult("path must be a .yaml or .yml file"), nil
	}

	// Read the file
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to read file: %v", err)), nil
	}

	rawConfig := string(buf)

	// Parse YAML
	parsedConfig, err := yaml.ParseString(rawConfig)
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed to parse YAML: %v", err)), nil
	}

	// Create WorkflowConfig
	config := &linter.WorkflowConfig{
		File:      path.Base(filePath),
		RawConfig: rawConfig,
		Workflow:  parsedConfig,
	}

	// Run linter
	strict := getBool(arguments, "strict", false)
	err = linter.New(
		linter.WithTrusted(linter.TrustedConfiguration{
			Network:  true,
			Volumes:  true,
			Security: true,
		}),
	).Lint([]*linter.WorkflowConfig{config})

	// Format the result
	var issues []map[string]interface{}
	var errorCount int
	var warningCount int

	if err != nil {
		pipelineErrors := errors.GetPipelineErrors(err)
		for _, pe := range pipelineErrors {
			issue := map[string]interface{}{
				"message":    pe.Message,
				"is_warning": pe.IsWarning,
				"type":       string(pe.Type),
			}

			// Extract field info based on error type
			if linterData := errors.GetLinterData(pe); linterData != nil {
				issue["file"] = linterData.File
				issue["field"] = linterData.Field
			} else if deprecationData, ok := pe.Data.(*errors.DeprecationErrorData); ok {
				issue["file"] = deprecationData.File
				issue["field"] = deprecationData.Field
				issue["docs"] = deprecationData.Docs
			} else if badHabitData, ok := pe.Data.(*errors.BadHabitErrorData); ok {
				issue["file"] = badHabitData.File
				issue["field"] = badHabitData.Field
				issue["docs"] = badHabitData.Docs
			}

			issues = append(issues, issue)

			if pe.IsWarning {
				warningCount++
			} else {
				errorCount++
			}
		}
	}

	// Determine overall status
	valid := true
	if err != nil {
		if strict && warningCount > 0 {
			valid = false
		} else if errorCount > 0 {
			valid = false
		}
	}

	response := map[string]interface{}{
		"valid":         valid,
		"path":          filePath,
		"error_count":   errorCount,
		"warning_count": warningCount,
		"issues":        issues,
		"strict":        strict,
	}

	if !valid {
		response["message"] = fmt.Sprintf("Config has %d error(s) and %d warning(s)", errorCount, warningCount)
	} else {
		response["message"] = "Config is valid"
		if warningCount > 0 {
			response["message"] = fmt.Sprintf("Config is valid with %d warning(s)", warningCount)
		}
	}

	// If strict mode caused failure due to warnings, add that info
	if strict && warningCount > 0 && errorCount == 0 {
		response["strict_failure"] = true
		response["message"] = "Config has warnings that are treated as errors in strict mode"
	}

	return tm.jsonResult(response)
}
