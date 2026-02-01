<p align="center">
  <img src="docs/assets/icon-small.png" alt="shelli" width="120">
</p>

<h1 align="center">shelli</h1>

<p align="center">
  <strong>Shell Interactive</strong> — Persistent shell sessions for AI agents
</p>

<p align="center">
  Enables persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.) that survive across CLI invocations.
</p>

<p align="center">
  <a href="https://github.com/schovi/shelli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/schovi/shelli/ci.yml?branch=main&style=flat-square" alt="Build"></a>
  <a href="https://github.com/schovi/shelli/releases"><img src="https://img.shields.io/github/v/release/schovi/shelli?style=flat-square" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
</p>

## Installation

### Homebrew (macOS/Linux)

```bash
brew install schovi/tap/shelli
```

### Go

```bash
go install github.com/schovi/shelli@latest
```

### From Source

```bash
git clone https://github.com/schovi/shelli
cd shelli
make build
```

## Claude Code Integration

shelli integrates with Claude Code in two ways: as a **Claude Plugin** (teaches Claude when and how to use shelli) and as an **MCP server** (provides native tool integration).

### MCP Server Setup

Add shelli as an MCP server in `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "shelli": {
      "command": "shelli",
      "args": ["daemon", "--mcp"]
    }
  }
}
```

This exposes 8 MCP tools to Claude:
- `shelli/create` - Create a new session
- `shelli/exec` - Send input and wait for output (primary tool)
- `shelli/send` - Send input without waiting
- `shelli/read` - Read session output
- `shelli/search` - Search output buffer with regex
- `shelli/list` - List all sessions
- `shelli/stop` - Stop session, keep output accessible
- `shelli/kill` - Stop and delete session

### Plugin Installation

Install the shelli plugin to teach Claude when to use persistent sessions:

```bash
claude plugins add schovi/shelli
```

The plugin includes:
- **Core skill**: Complete shelli command reference, escape sequences, best practices
- **Auto-detector**: Recognizes when shelli is needed (SSH, REPLs, databases, stateful workflows)
- **`/shelli` command**: Explicit entry point for forcing shelli usage

With both MCP and plugin installed, Claude will:
1. Automatically detect when persistent sessions are needed
2. Use MCP tools for structured interaction
3. Handle session lifecycle (create, use, cleanup)

### Example Interaction

```
User: "SSH to server.example.com and check disk usage"

Claude: [creates SSH session via MCP, waits for prompt, runs df -h]
```

```
User: "Start Python and help me explore this CSV"

Claude: [creates python3 session, imports pandas, loads file interactively]
```

## Commands

### create

Create a new interactive session.

```bash
shelli create <name> [--cmd "command"] [--json]
```

Examples:
```bash
shelli create myshell                        # default shell
shelli create pyrepl --cmd "python3"         # Python REPL
shelli create db --cmd "psql -d mydb"        # PostgreSQL
shelli create server --cmd "ssh user@host"   # SSH session
```

### exec

Send input and wait for result. The primary command for AI agents.

```bash
shelli exec <name> <input> [flags]
```

Flags:
- `--settle N` - Wait for N ms of silence (default: 500)
- `--wait "pattern"` - Wait for regex pattern match (mutually exclusive with --settle)
- `--timeout N` - Max wait time in seconds (default: 10)
- `--strip-ansi` - Remove terminal escape codes
- `--json` - Output as JSON

Examples:
```bash
shelli exec pyrepl "print('hello')"                # wait for output to settle
shelli exec pyrepl "print('hello')" --settle 1000  # longer settle
shelli exec myshell "ls" --wait '\$'               # wait for shell prompt
shelli exec db "SELECT 1;" --strip-ansi --json     # clean JSON output
```

### send

Send input to a session. Appends newline by default.

```bash
shelli send <name> <input> [--raw]
```

**Normal mode** (default): Appends newline, sends as-is.

**Raw mode** (`--raw`): No newline, interprets escape sequences.

Examples:
```bash
shelli send myshell "ls -la"           # send command + newline
shelli send pyrepl "print('hello')"    # send to Python + newline
shelli send myshell "\x03" --raw       # send Ctrl+C
shelli send myshell "\x04" --raw       # send Ctrl+D (EOF)
shelli send myshell "y" --raw          # send 'y' without newline
```

### read

Read output from a session.

```bash
shelli read <name> [flags]
```

**Instant modes** (non-blocking):
- (default) - New output since last read
- `--all` - All output from session start

**Blocking modes** (returns new output):
- `--wait "pattern"` - Wait for regex pattern match
- `--settle N` - Wait for N ms of silence

Other flags:
- `--timeout N` - Max wait time in seconds (default: 10)
- `--strip-ansi` - Remove terminal escape codes
- `--json` - Output as JSON

Examples:
```bash
shelli read myshell                    # new output, instant
shelli read myshell --all              # all output, instant
shelli read pyrepl --wait ">>>"        # wait for Python prompt
shelli read myshell --settle 300       # wait for 300ms silence
```

### search

Search session output buffer for regex patterns.

```bash
shelli search <name> <pattern> [flags]
```

Flags:
- `--before N` - Lines of context before match
- `--after N` - Lines of context after match
- `--around N` - Lines of context before and after
- `--ignore-case` - Case-insensitive search
- `--strip-ansi` - Strip ANSI codes before searching
- `--json` - Output as JSON

Examples:
```bash
shelli search myshell "error"                    # find errors
shelli search myshell "ERROR|WARN" --around 3    # with context
shelli search db "SELECT" --ignore-case          # case-insensitive
```

### list

List all sessions with their state.

```bash
shelli list [--json]
```

Output shows: `name`, `state` (running/stopped), `pid`, `command`

### stop

Stop a running session but keep output accessible.

```bash
shelli stop <name>
```

The process is terminated (SIGTERM → SIGKILL) but:
- Output remains readable via `read` and `search`
- Session stays in `list` with state `stopped`
- Use `kill` to fully remove

### kill

Stop and delete a session completely.

```bash
shelli kill <name>
```

This is a compound operation:
- If running: stops the process first
- Deletes all session data (output and metadata)

## Session Lifecycle

Sessions have explicit states with clear transitions:

```
     create
        ↓
    [running] ←→ exec/send/read/search
        ↓
      stop (or natural exit)
        ↓
    [stopped] ←→ read/search only
        ↓
      kill
        ↓
    (removed)
```

- **running**: Process is active, all commands work
- **stopped**: Process terminated, output preserved for reading
- Stopped sessions reject `exec` and `send` with an error

## Storage

By default, shelli stores session output in files at `/tmp/shelli/`:

```
/tmp/shelli/
├── mysession.out    # raw PTY output
└── mysession.meta   # JSON metadata (state, pid, timestamps)
```

This means:
- **Output survives daemon restart** - stopped sessions are recovered
- **Unlimited output size** - no buffer limits
- **Persistent read position** - continues where you left off

### Daemon Flags

```bash
shelli daemon [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/tmp/shelli` | Directory for session files |
| `--memory-backend` | `false` | Use in-memory storage (no persistence) |
| `--stopped-ttl` | (disabled) | Auto-delete stopped sessions after duration |
| `--max-output` | `10MB` | Buffer size limit (memory backend only) |

Examples:
```bash
# Use custom storage location
shelli daemon --data-dir ~/.shelli/sessions

# Memory-only mode (v0.3 behavior)
shelli daemon --memory-backend --max-output 50MB

# Auto-cleanup stopped sessions after 1 hour
shelli daemon --stopped-ttl 1h
```

## Escape Sequences

When using `send --raw`, escape sequences are interpreted:

| Sequence | Character | Description |
|----------|-----------|-------------|
| `\x00`-`\xFF` | Any byte | Hex byte value |
| `\n` | LF | Newline |
| `\r` | CR | Carriage return |
| `\t` | Tab | Horizontal tab |
| `\e` | ESC | Escape (ASCII 27) |
| `\\` | \ | Literal backslash |
| `\0` | NUL | Null byte |

### Common Control Characters

| Sequence | Key | Effect |
|----------|-----|--------|
| `\x03` | Ctrl+C | Interrupt (SIGINT) |
| `\x04` | Ctrl+D | End of file (EOF) |
| `\x1a` | Ctrl+Z | Suspend (SIGTSTP) |
| `\x1c` | Ctrl+\ | Quit (SIGQUIT) |
| `\x0c` | Ctrl+L | Clear screen |
| `\x09` | Ctrl+I / Tab | Tab character |

### Examples

```bash
# Interrupt a long-running command
shelli send myshell "\x03" --raw

# Send EOF to close stdin
shelli send myshell "\x04" --raw

# Tab completion
shelli send myshell "doc\t" --raw

# Answer a yes/no prompt without newline, then send newline
shelli send myshell "y" --raw
shelli send myshell ""              # just newline
```

## Architecture

shelli uses a daemon process to maintain PTY handles across CLI invocations:

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Claude Code                                  │
│  Tool call: shelli/exec                                             │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ JSON-RPC over stdio (MCP)
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      shelli daemon                                   │
│                                                                      │
│  ┌────────────────────┐      ┌────────────────────────────────┐    │
│  │ MCP Server         │      │ Socket Server                  │    │
│  │ (--mcp flag)       │      │ (~/.shelli/shelli.sock)        │    │
│  └─────────┬──────────┘      └─────────────┬──────────────────┘    │
│            └───────────┬───────────────────┘                        │
│                        ▼                                            │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ Session Manager                                              │   │
│  │ PTY sessions accessible via both MCP and CLI                 │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                               ▲
                               │ Unix socket
┌──────────────────────────────┴──────────────────────────────────────┐
│                         shelli CLI                                   │
│  $ shelli create / exec / send / read / search / list / stop / kill │
└─────────────────────────────────────────────────────────────────────┘
```

- First CLI command auto-starts the daemon if not running
- Daemon manages PTY handles and session state
- Sessions are shared between MCP and CLI
- Output stored in files (default) or memory, with read position tracking
- Stopped sessions recovered on daemon restart (file backend only)

## Typical Workflow

```bash
# Create a Python REPL session
shelli create py --cmd "python3"

# Execute commands
shelli exec py "x = 42"
shelli exec py "print(x * 2)" --strip-ansi
# Output: 84

# Stop session but keep output
shelli stop py
shelli read py --all        # still works!

# Fully remove when done
shelli kill py
```

## Limitations

### TUI Applications

shelli does **not** support full-screen TUI applications like `k9s`, `btop`, `htop`, `vim`, `nano`, etc.

These apps paint 2D screens using cursor positioning, not line-based output.

**Workarounds:**
- `k9s` → `kubectl get pods`, `kubectl describe pod`
- `btop`/`htop` → `ps aux`, `top -bn1`
- `vim` → `sed`, `awk`, or direct file manipulation

**shelli works best with:**
- REPLs (Python, Node, Ruby, etc.)
- Database CLIs (psql, mysql, sqlite3)
- SSH sessions
- Any tool that produces line-based text output

## Development

```bash
make build      # Build binary
make test       # Run tests
make lint       # Run golangci-lint
make security   # Run gosec + govulncheck
```

## Version

v0.4.0
