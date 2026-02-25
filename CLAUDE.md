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
- Commands: create, exec, send, read, list, stop, kill, search, cursor, version, daemon

**Utilities** (`internal/`)
- `wait/`: Output polling with settle-time and pattern-matching modes. Supports `FullOutput` flag for TUI sessions where output is full screen content rather than a growing buffer.
- `vterm/`: VT terminal emulator wrapper using `charmbracelet/x/vt` (see `docs/TUI.md` for details)
  - `screen.go`: `Screen` wraps a thread-safe VT emulator with atomic version counter and terminal query response bridge. Used for TUI sessions (replaces raw byte storage + frame detection + terminal responder).
  - `strip.go`: ANSI escape code removal. Detects cursor positioning sequences and uses a temporary VT emulator for correct rendering; falls back to fast regex stripping for simple output.
- `escape/`: Escape sequence interpretation for raw mode

### Data Flow

```
CLI/MCP → daemon.Client → Unix socket → daemon.Server → PTY → subprocess
                                              ↓
                                    ┌─── TUI sessions ───┐
                                    │  vterm.Screen       │
                                    │  (VT emulator IS    │
                                    │   the screen state) │
                                    └─────────────────────┘
                                    ┌─── Non-TUI sessions ┐
                                    │  OutputStorage       │
                                    │  ├─ MemoryStorage    │
                                    │  └─ FileStorage      │
                                    └──────────────────────┘
```

PTY sessions accessible via both MCP and CLI, with optional size-based poll optimization. Additional endpoints: `size` (returns version counter for TUI, byte count for non-TUI)

### Key Design Decisions

- **Daemon holds state**: PTY file descriptors can't be passed across processes, so a long-running daemon is required
- **Two interfaces**: CLI commands for users/testing, MCP for AI agent integration
- **Settle vs wait modes**: `--settle` waits for silence, `--wait` matches regex patterns
- **Read position tracking**: Each session tracks where the last read ended
- **Storage abstraction**: Pluggable backends allow testing with memory, persistence with files
- **Stop vs Kill**: `stop` terminates process but keeps output accessible; `kill` deletes everything
- **Session states**: Sessions can be "running" or "stopped" with timestamp tracking
- **TTL cleanup**: Optional auto-deletion of stopped sessions via `--stopped-ttl`
- **TUI mode with VT emulator**: `--tui` flag creates a `vterm.Screen` (VT emulator) for the session. PTY output feeds the emulator directly; no raw byte storage needed. The emulator handles all cursor positioning, screen clearing, and character rendering natively. Reads return the current screen state via `Render()` (ANSI) or `String()` (plain text).
- **VT emulator response bridge**: The emulator automatically handles terminal capability queries (DA1, DA2, DSR, etc.) and writes responses to its internal pipe. A `ReadResponses` goroutine bridges these to the PTY master, unblocking apps like yazi.
- **Snapshot read**: `--snapshot` triggers a resize cycle (SIGWINCH) to force a full TUI redraw, waits for the emulator version to settle, then reads `screen.String()` (plain text). No storage clearing or frame detection needed.
- **Per-consumer cursors**: Optional `cursor` parameter on read operations. Each named cursor tracks its own read position (byte offset for non-TUI, version counter for TUI), allowing multiple consumers to tail the same session independently. Without a cursor, the global `ReadPos` is used (backward compatible).
- **Size endpoint**: Lightweight `size` action returns version counter (TUI) or buffer byte count (non-TUI). Used by wait polling to skip expensive full reads when nothing changed.

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
- **Tests**: `internal/vterm/strip_test.go`, `internal/vterm/screen_test.go`, `internal/wait/wait_test.go`, `internal/daemon/limitlines_test.go`
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
