# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
make build      # Build binary
make run        # Run directly (go run .)
make test       # Run tests
make lint       # Run golangci-lint
make security   # Run gosec + govulncheck
make install    # Install globally
```

Or without Make:
```bash
go build -o shelli .
go test -v -race ./...
```

## Architecture

shelli provides persistent interactive shell sessions via PTY-backed processes managed by a daemon.

### Components

**Daemon** (`internal/daemon/`)
- `server.go`: Session manager holding PTY handles, output buffers, and process state
- `client.go`: Unix socket client for CLI-to-daemon communication
- Socket at `~/.shelli/shelli.sock`, auto-started on first command

**MCP Server** (`internal/mcp/`)
- `server.go`: JSON-RPC stdio server implementing MCP protocol
- `tools.go`: Tool registry exposing create/exec/send/read/list/kill operations
- Started via `shelli daemon --mcp`

**CLI** (`cmd/`)
- Cobra commands wrapping client calls
- Each command (create, exec, send, read, list, kill) maps to a daemon action

**Utilities** (`internal/`)
- `wait/`: Output polling with settle-time and pattern-matching modes
- `ansi/`: ANSI escape code stripping
- `escape/`: Escape sequence interpretation for raw mode

### Data Flow

```
CLI/MCP → daemon.Client → Unix socket → daemon.Server → PTY → subprocess
                                              ↓
                                        output buffer (with read position tracking)
```

### Key Design Decisions

- **Daemon holds state**: PTY file descriptors can't be passed across processes, so a long-running daemon is required
- **Two interfaces**: CLI commands for users/testing, MCP for AI agent integration
- **Settle vs wait modes**: `--settle` waits for silence, `--wait` matches regex patterns
- **Read position tracking**: Each session tracks where the last read ended

## Claude Plugin

`.claude-plugin/` contains plugin metadata. The plugin teaches Claude when to use shelli (SSH, REPLs, databases, stateful workflows).

Skills in `skills/`:
- `shelli/SKILL.md`: Full command reference
- `shelli-auto-detector/SKILL.md`: Pattern detection for automatic usage

## Tooling

- **Linting**: `.golangci.yml` - golangci-lint config with gosec, gocritic, revive
- **CI/CD**: `.github/workflows/ci.yml` - lint, test, build, security on push/PR
- **Releases**: `.goreleaser.yml` - multi-platform binaries, Homebrew tap update on tags
- **Tests**: `internal/ansi/strip_test.go`, `internal/wait/wait_test.go`
- **Version**: `shelli version` - build info injected by goreleaser
