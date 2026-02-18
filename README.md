<p align="center">
  <img src="docs/assets/icon-small.png" alt="shelli" width="120">
</p>

<h1 align="center">shelli</h1>

<p align="center">
  <strong>Shell Interface</strong> — Persistent shell sessions for AI agents
</p>

<p align="center">
  Enables persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.) that survive across CLI invocations.
</p>

<p align="center">
  <a href="https://github.com/schovi/shelli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/schovi/shelli/ci.yml?branch=main&style=flat-square" alt="Build"></a>
  <a href="https://github.com/schovi/shelli/releases"><img src="https://img.shields.io/github/v/release/schovi/shelli?style=flat-square" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
</p>

> **Note:** This project is in active development. The API may change until v1.0.0.

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

This exposes 11 MCP tools to Claude:
- `shelli/create` - Create a new session
- `shelli/exec` - Send input and wait for output (primary tool)
- `shelli/send` - Send input without waiting
- `shelli/read` - Read session output
- `shelli/search` - Search output buffer with regex
- `shelli/list` - List all sessions
- `shelli/info` - Get detailed session info
- `shelli/clear` - Clear output buffer
- `shelli/resize` - Change terminal dimensions
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
shelli create <name> [flags]
```

Flags:
- `--cmd "command"` - Command to run (default: $SHELL)
- `--env KEY=VALUE` - Set environment variable (repeatable)
- `--cwd /path` - Set working directory
- `--cols N` - Terminal columns (default: 80)
- `--rows N` - Terminal rows (default: 24)
- `--tui` - Enable TUI mode (auto-truncate buffer on frame boundaries)
- `--json` - Output as JSON

Examples:
```bash
shelli create myshell                        # default shell
shelli create pyrepl --cmd "python3"         # Python REPL
shelli create db --cmd "psql -d mydb"        # PostgreSQL
shelli create server --cmd "ssh user@host"   # SSH session
shelli create dev --env "DEBUG=1" --cwd /app # with env and cwd
shelli create wide --cols 200 --rows 50      # large terminal
shelli create vim --cmd "vim" --tui          # TUI mode for editors
```

### exec

Send a command and wait for result. The primary command for AI agents.

```bash
shelli exec <name> <input> [flags]
```

Input is sent as literal text with a newline appended. Escape sequences like `\n` are NOT interpreted by shelli - they're passed to the shell as-is (the shell may interpret them, e.g., `echo -e`).

For precise control over escape sequences or TUI apps, use `send` instead.

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
shelli exec myshell "echo -e 'hello\nworld'"       # \n passed to shell's echo
```

### send

Send raw input to a session. Low-level command for precise control.

```bash
shelli send <name> <input> [input...]
```

- Each argument is sent as a separate write to the PTY
- Escape sequences are always interpreted
- No newline is added automatically

Examples:
```bash
shelli send myshell "ls -la\n"          # command with explicit newline
shelli send tui "hello" "\r"            # TUI: type "hello", then Enter (separate writes)
shelli send tui "hello\r"               # TUI: same but single write
shelli send myshell "\x03"              # send Ctrl+C
shelli send myshell "\x04"              # send Ctrl+D (EOF)
shelli send myshell "y"                 # send 'y' without newline
```

**MCP: Special characters and `input_base64`**

When using MCP tools, characters like `!` can cause bash history expansion issues. For inputs with special characters or binary data, use `input_base64`:

```json
// Avoids bash escaping issues with "!"
{"name": "session", "input_base64": "SGVsbG8gWmVwaHlyIQ=="}
```

The `inputs` array is preferred for multi-step sequences (e.g., message + Enter):
```json
{"name": "session", "inputs": ["Hello", "\r"]}
```

### read

Read output from a session.

```bash
shelli read <name> [flags]
```

**Instant modes** (non-blocking):
- (default) - New output since last read
- `--all` - All output from session start

**Streaming mode**:
- `--follow` / `-f` - Continuous output like `tail -f` (great for TUIs)
- `--follow-ms N` - Poll interval in milliseconds (default: 100)

**Snapshot mode** (TUI only):
- `--snapshot` - Force a full redraw via resize, wait for settle, read clean frame

**Blocking modes** (returns new output):
- `--wait "pattern"` - Wait for regex pattern match
- `--settle N` - Wait for N ms of silence
- `--head N` / `--tail N` - Limit output lines (applied after wait/settle completes)

Other flags:
- `--timeout N` - Max wait time in seconds (default: 10)
- `--settle N` - Override default settle time (300ms for snapshot, used with --wait/--settle modes)
- `--strip-ansi` - Remove terminal escape codes
- `--cursor "name"` - Named cursor for per-consumer read tracking
- `--json` - Output as JSON

Examples:
```bash
shelli read myshell                    # new output, instant
shelli read myshell --all              # all output, instant
shelli read pyrepl --wait ">>>"        # wait for Python prompt
shelli read myshell --settle 300       # wait for 300ms silence
shelli read tui-app --snapshot --strip-ansi  # clean TUI frame
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

### info

Get detailed information about a session.

```bash
shelli info <name> [--json]
```

Shows: name, state, pid, command, created_at, stopped_at (if stopped), uptime, buffer size, read position, terminal dimensions.

### clear

Clear the output buffer of a session.

```bash
shelli clear <name> [--json]
```

Truncates the output buffer and resets the read position. The session continues running.

### resize

Change terminal dimensions of a running session.

```bash
shelli resize <name> [--cols N] [--rows N] [--json]
```

At least one of `--cols` or `--rows` must be specified. Omitted dimensions keep their current value.

Examples:
```bash
shelli resize myshell --cols 120 --rows 40   # set both
shelli resize myshell --cols 200             # change only width
```

### stop

Stop a running session but keep output accessible.

```bash
shelli stop <name> [--json]
```

The process is terminated (SIGTERM → SIGKILL) but:
- Output remains readable via `read` and `search`
- Session stays in `list` with state `stopped`
- Use `kill` to fully remove

### kill

Stop and delete a session completely.

```bash
shelli kill <name> [--json]
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

By default, shelli stores session output in files at `~/.shelli/data/`:

```
~/.shelli/data/
├── mysession.out    # raw PTY output (0600 permissions)
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
| `--data-dir` | `~/.shelli/data` | Directory for session files |
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

When using `send`, escape sequences are always interpreted:

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
shelli send myshell "\x03"

# Send EOF to close stdin
shelli send myshell "\x04"

# Tab completion
shelli send myshell "doc\t"

# Answer a yes/no prompt without newline, then send newline
shelli send myshell "y"
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
│  $ shelli create / exec / send / read / search / list / info /       │
│  $         clear / resize / stop / kill                              │
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

shelli supports TUI applications using `--follow` mode, `--tui` mode for buffer management, and `--snapshot` for clean frame capture:

```bash
shelli create mon --cmd "btop" --tui   # TUI mode auto-truncates buffer
shelli read mon --follow               # streams output continuously
shelli read mon --snapshot --strip-ansi  # force redraw, get clean frame
```

**TUI Mode (`--tui` flag):**

When enabled, shelli uses multiple detection strategies to identify frame boundaries and truncate old content:

| Strategy | Trigger | Apps |
|----------|---------|------|
| Screen clear | ESC[2J, ESC[?1049h, ESC c | vim, less, nano |
| Sync mode | ESC[?2026h (begin) | Claude Code, modern terminals |
| Cursor home | ESC[1;1H (with reset) | k9s, btm, htop |
| Size cap | Buffer > 100KB after frame | Fallback after frame detection |

This reduces storage from ~50MB to ~2KB for typical TUI sessions.

**What works well** (9/9 test score):
- System monitors: `btop`, `htop`, `glances`, `k9s`
- File managers: `ranger`, `nnn`, `yazi`, `vifm`, `mc`
- Git tools: `lazygit`, `tig`
- Editors/viewers: `vim`, `less`, `micro`, `bat`
- Chat clients: `weechat`, `irssi`, `newsboat`

See [docs/TUI.md](docs/TUI.md) for detailed TUI internals, app compatibility, and known limitations.

Apps with complex mouse/input handling may behave unexpectedly.

**shelli works best with:**
- REPLs (Python, Node, Ruby, etc.)
- Database CLIs (psql, mysql, sqlite3)
- SSH sessions
- TUI monitors and dashboards (with `--follow` and `--tui`)

## Development

```bash
make build      # Build binary
make test       # Run tests
make lint       # Run golangci-lint
make security   # Run gosec + govulncheck
```

## Contributing

Contributions welcome. Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Run tests and linting before committing (`make test && make lint`)
4. Open a pull request against `main`

For bugs or feature requests, open an issue first to discuss.
