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
- `server.go`: Session manager with PTY handles, session state, and process lifecycle
- `client.go`: Unix socket client for CLI-to-daemon communication
- `storage.go`: `OutputStorage` interface for pluggable backends
- `storage_memory.go`: In-memory storage with circular buffer (default, 10MB limit)
- `storage_file.go`: File-based persistent storage
- `constants.go`: Shared constants (buffer sizes, timeouts)
- Socket at `~/.shelli/shelli.sock`, auto-started on first command

**MCP Server** (`internal/mcp/`)
- `server.go`: JSON-RPC stdio server implementing MCP protocol
- `tools.go`: Tool registry exposing 8 operations: create/exec/send/read/list/stop/kill/search
- Started via `shelli daemon --mcp`

**CLI** (`cmd/`)
- Cobra commands wrapping client calls
- Commands: create, exec, send, read, list, stop, kill, search, version, daemon

**Utilities** (`internal/`)
- `wait/`: Output polling with settle-time and pattern-matching modes
- `ansi/`: ANSI escape code stripping, TUI frame detection, terminal query responses (see `docs/TUI.md` for details)
  - `strip.go`: ANSI escape code removal with rune-based virtual screen buffer supporting cursor positioning, relative movement (A/B/C/D), erase line (K), DEC Special Graphics charset, and newline-based grid sizing
  - `clear.go`: `FrameDetector` for TUI mode (screen clear, sync mode, cursor home with cooldown, CursorJumpTop with look-ahead, size cap). Snapshot mode suppresses ALL truncation strategies.
  - `responder.go`: `TerminalResponder` intercepts DA1/DA2/Kitty queries and writes responses to PTY
- `escape/`: Escape sequence interpretation for raw mode

### Data Flow

```
CLI/MCP → daemon.Client → Unix socket → daemon.Server → PTY → subprocess
                                              ↓
                                        OutputStorage
                                        ├─ MemoryStorage (default)
                                        └─ FileStorage (persistent)
```

### Key Design Decisions

- **Daemon holds state**: PTY file descriptors can't be passed across processes, so a long-running daemon is required
- **Two interfaces**: CLI commands for users/testing, MCP for AI agent integration
- **Settle vs wait modes**: `--settle` waits for silence, `--wait` matches regex patterns
- **Read position tracking**: Each session tracks where the last read ended
- **Storage abstraction**: Pluggable backends allow testing with memory, persistence with files
- **Stop vs Kill**: `stop` terminates process but keeps output accessible; `kill` deletes everything
- **Session states**: Sessions can be "running" or "stopped" with timestamp tracking
- **TTL cleanup**: Optional auto-deletion of stopped sessions via `--stopped-ttl`
- **TUI mode**: `--tui` flag enables frame detection with multiple strategies (screen clear, sync mode, cursor home, size cap) to auto-truncate buffer for TUI apps
- **Snapshot read**: `--snapshot` on read clears storage and resets the frame detector, then triggers a resize cycle (SIGWINCH) to force a full TUI redraw, waits for settle, then reads the clean frame. Pre-clearing prevents races between captureOutput and the settle loop. Requires TUI mode.
- **Terminal responder**: TUI sessions get a `TerminalResponder` that intercepts terminal capability queries (DA1, DA2, Kitty keyboard, DECRPM) in PTY output and writes responses to PTY input. Unblocks apps like yazi that block on unanswered queries.

## Claude Plugin

`.claude/.claude-plugin/` contains plugin metadata. The plugin teaches Claude when to use shelli (SSH, REPLs, databases, stateful workflows).

Skills in `.claude/skills/`:
- `shelli/SKILL.md`: Full command reference
- `shelli-auto-detector/SKILL.md`: Pattern detection for automatic usage
- `tui-test/SKILL.md`: Automated TUI app testing protocol

## Tooling

- **Linting**: `.golangci.yml` - golangci-lint config with gosec, gocritic, revive
- **CI/CD**: `.github/workflows/ci.yml` - lint, test, build, security on push/PR
- **Releases**: `.goreleaser.yml` - multi-platform binaries, Homebrew tap update on tags
- **Tests**: `internal/ansi/strip_test.go`, `internal/ansi/clear_test.go`, `internal/wait/wait_test.go`, `internal/daemon/limitlines_test.go`
- **Version**: `shelli version` - build info injected by goreleaser

## Documentation Sync Rules

When making changes, keep documentation in sync across these files:

| Change Type | Update These Files |
|-------------|-------------------|
| New CLI command/flag | README.md, `.claude/skills/shelli/SKILL.md` |
| New MCP tool/parameter | README.md, `.claude/skills/shelli/SKILL.md`, `internal/mcp/tools.go` schema |
| Architecture change | CLAUDE.md, README.md (if user-facing) |
| New internal component | CLAUDE.md |
| Plugin behavior change | `.claude/skills/shelli-auto-detector/SKILL.md` |
| CLI/MCP interface change | `.claude/skills/tui-test/SKILL.md` (update test protocol, app registry, and commands to match new interface) |
| TUI behavior change | `docs/TUI.md`, CLAUDE.md |

**Rule**: After any feature or architecture change, update CLAUDE.md to reflect the current state. CLAUDE.md should always accurately describe the codebase structure.
