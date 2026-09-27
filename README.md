# Woodpecker CI MCP Server

A Model Context Protocol (MCP) server that provides AI agents with access to Woodpecker CI build statuses, pipeline management, and repository information.

## Features

- **Pipeline Management**: List, start, stop, and approve pipelines
- **Build Status Monitoring**: Get real-time pipeline statuses and build information
- **Repository Management**: List and manage repositories
- **Log Viewing**: Retrieve pipeline and step logs
- **SSH Compatible**: Token-based authentication for remote usage
- **Comprehensive CLI**: Easy setup and management with styled terminal interface

## Available MCP Tools

### Pipeline Management
- `list_pipelines` - List pipelines for a repository
- `get_pipeline_status` - Get the status of a specific pipeline
- `start_pipeline` - Start (restart) a specific pipeline
- `stop_pipeline` - Stop a running pipeline
- `wait_for_pipeline` - Block until a pipeline finishes (default timeout 2h), then return one
  compact summary: outcome, duration, and, on failure, the log tail of each failed step.
  Select the pipeline by `pipeline_number`, `latest`, or `commit` (waits for the pipeline to
  appear, useful right after a push). Returns early on `blocked` unless `wait_on_blocked` is set.
  Sends MCP progress notifications when the client supplies a progress token. At most 3 waits
  run concurrently so the stdio worker pool stays available for other tools.
- `approve_pipeline` - Approve a pending pipeline

### Repository Management
- `list_repositories` - List all accessible repositories
- `get_repository` - Get detailed repository information

### Log Management
- `get_logs` - Get logs for a specific pipeline step

## Installation

### Prerequisites

- Go 1.24.6 or later
- Access to a Woodpecker CI server
- Personal Access Token from Woodpecker CI

### Build from Source

```bash
git clone https://github.com/denysvitali/woodpecker-ci-mcp
cd woodpecker-ci-mcp
go build -o woodpecker-mcp ./cmd/woodpecker-mcp
```

### Install

```bash
# Install directly from source
go install github.com/denysvitali/woodpecker-ci-mcp/cmd/woodpecker-mcp@latest
```

## Quick Start

### 1. Initial Setup

Run the interactive configuration setup:

```bash
woodpecker-mcp config init
```

This will guide you through:
- Setting up your Woodpecker CI server URL
- Configuring your Personal Access Token
- Saving the configuration securely

### 2. Test Connection

Verify your setup works:

```bash
woodpecker-mcp test
```

### 3. Start the MCP Server

```bash
woodpecker-mcp serve
```

The server will start and listen for MCP requests via stdio.

## Configuration

### Configuration File

The default configuration file is located at `~/.config/woodpecker-mcp/config.yaml`:

```yaml
# Server configuration
server:
  name: "woodpecker-mcp"
  version: "1.0.0"

# Woodpecker CI connection settings
woodpecker:
  url: "https://woodpecker.example.com"
  token: "your-personal-access-token-here"

# Logging configuration
logging:
  level: "info"  # debug, info, warn, error
  format: "text" # text, json
```

### Environment Variables

You can also configure using environment variables (prefixed with `WOODPECKER_MCP_`):

```bash
export WOODPECKER_MCP_WOODPECKER_URL="https://woodpecker.example.com"
export WOODPECKER_MCP_WOODPECKER_TOKEN="your-token-here"
export WOODPECKER_MCP_LOGGING_LEVEL="info"
export WOODPECKER_MCP_LOGGING_FORMAT="text"
```

### Getting a Personal Access Token

1. Open your Woodpecker CI server in a web browser
2. Log in to your account
3. Click on your user icon in the top right corner
4. Go to your personal profile page
5. Generate a new Personal Access Token
6. Copy the token and use it in configuration

## CLI Commands

### Server Commands

```bash
# Start the MCP server (default command)
woodpecker-mcp serve

# Start with custom config file
woodpecker-mcp serve --config /path/to/config.yaml

# Start with debug logging
woodpecker-mcp serve --log-level debug
```

### Configuration Commands

```bash
# Interactive setup
woodpecker-mcp config init

# Show current configuration
woodpecker-mcp config show
```

### Authentication Commands

```bash
# Interactive token setup
woodpecker-mcp auth login

# Test connection
woodpecker-mcp test
```

## MCP Client Configuration

### Claude Desktop

Add to your Claude Desktop configuration (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "woodpecker": {
      "command": "woodpecker-mcp",
      "args": ["serve"]
    }
  }
}
```

### Other MCP Clients

The server communicates via stdio, so it can be used with any MCP-compatible client:

```bash
# Direct execution
woodpecker-mcp serve

# With custom configuration
WOODPECKER_MCP_WOODPECKER_URL=https://your-server.com woodpecker-mcp serve
```

## Usage Examples

Once connected through an MCP client, you can use these tools:

### List Repositories
```json
{
  "tool": "list_repositories",
  "arguments": {
    "all": false
  }
}
```

### Get Pipeline Status
```json
{
  "tool": "get_pipeline_status",
  "arguments": {
    "repo_name": "owner/repository",
    "latest": true
  }
}
```

### Start a Pipeline
```json
{
  "tool": "start_pipeline",
  "arguments": {
    "repo_name": "owner/repository",
    "pipeline_number": 123
  }
}
```

### Get Logs
```json
{
  "tool": "get_logs",
  "arguments": {
    "repo_name": "owner/repository",
    "pipeline_number": 123,
    "step_id": 1,
    "format": "text"
  }
}
```

## SSH Usage

This MCP server is designed to work over SSH connections. The token-based authentication means you don't need browser access on the remote machine:

1. Configure the server on your local machine or remote server
2. Use environment variables or config files for authentication
3. The server works entirely through the command line interface

## Development

### Project Structure

```
woodpecker-ci-mcp/
├── cmd/woodpecker-mcp/    # Main CLI application
├── internal/
│   ├── config/            # Configuration management
│   ├── auth/              # Authentication handling  
│   ├── client/            # Woodpecker client wrapper
│   └── server/            # MCP server implementation
├── tools/                 # MCP tool implementations
│   ├── pipelines.go       # Pipeline management tools
│   ├── repositories.go    # Repository tools
│   └── logs.go           # Log viewing tools
└── README.md
```

### Dependencies

- [mcp-go](https://github.com/mark3labs/mcp-go) - MCP server implementation
- [woodpecker-go](https://go.woodpecker-ci.org/woodpecker/v2) - Woodpecker CI client
- [cobra](https://github.com/spf13/cobra) - CLI framework
- [viper](https://github.com/spf13/viper) - Configuration management
- [lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [logrus](https://github.com/sirupsen/logrus) - Structured logging

### Building

```bash
# Build for current platform
go build -o woodpecker-mcp ./cmd/woodpecker-mcp

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o woodpecker-mcp ./cmd/woodpecker-mcp
```

### Testing

```bash
# Run tests
go test ./...

# Test with coverage
go test -cover ./...
```

## Troubleshooting

### Connection Issues

1. **Invalid URL**: Ensure your Woodpecker server URL is correct and accessible
2. **Authentication Failed**: Verify your Personal Access Token is valid and has proper permissions
3. **Network Issues**: Check firewall and network connectivity to your Woodpecker server

### Configuration Issues

1. **Config Not Found**: Run `woodpecker-mcp config init` to create initial configuration
2. **Permission Denied**: Ensure the config directory (`~/.config/woodpecker-mcp`) is writable
3. **Invalid Token**: Use `woodpecker-mcp auth login` to update your authentication token

### Logging

Enable debug logging for troubleshooting:

```bash
woodpecker-mcp serve --log-level debug
```

Or set via environment variable:

```bash
export WOODPECKER_MCP_LOGGING_LEVEL=debug
woodpecker-mcp serve
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Commit your changes: `git commit -m 'Add amazing feature'`
4. Push to the branch: `git push origin feature/amazing-feature`
5. Open a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Woodpecker CI](https://woodpecker-ci.org/) for the excellent CI/CD platform
- [Model Context Protocol](https://modelcontextprotocol.io/) for the standardized interface
- [mcp-go](https://mcp-go.dev/) for the Go SDK implementation