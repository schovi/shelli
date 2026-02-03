# shelli - Persistent Interactive Shell Sessions

shelli enables persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.) that survive across CLI invocations. This skill provides comprehensive knowledge for using shelli effectively.

## MCP Integration

**If MCP shelli tools are available** (check for `shelli/create`, `shelli/exec`, etc.), prefer using them over Bash commands. MCP tools provide structured responses and better error handling.

MCP tools map directly to CLI commands:
- `shelli/create` → `shelli create`
- `shelli/exec` → `shelli exec`
- `shelli/send` → `shelli send`
- `shelli/read` → `shelli read`
- `shelli/search` → `shelli search`
- `shelli/list` → `shelli list`
- `shelli/info` → `shelli info`
- `shelli/clear` → `shelli clear`
- `shelli/resize` → `shelli resize`
- `shelli/stop` → `shelli stop`
- `shelli/kill` → `shelli kill`

If MCP tools are not available, use the Bash commands documented below.

## When to Use shelli

Use shelli instead of regular Bash when you need:

- **State persistence**: Variables, session state, or context must survive between commands
- **Interactive prompts**: REPLs that wait for input (Python `>>>`, Node `>`, etc.)
- **Remote sessions**: SSH connections where you run multiple commands
- **Database CLIs**: psql, mysql, sqlite3, mongosh, redis-cli
- **Multi-step workflows**: Sequential operations that depend on prior state
- **Long-running processes**: Servers, watchers, or processes you need to interact with

Do NOT use shelli for:
- One-off commands that don't need state (`ls`, `cat file.txt`, `git status`)
- Commands that complete immediately and don't require follow-up
- TUI applications (vim, htop, k9s) - they don't produce line-based output

## Commands Reference

### create - Create a new session

```bash
shelli create <name> [flags]
```

Flags:
- `--cmd "command"`: Command to run (default: user's shell)
- `--env KEY=VALUE`: Set environment variable (repeatable)
- `--cwd /path`: Set working directory
- `--cols N`: Terminal columns (default: 80)
- `--rows N`: Terminal rows (default: 24)
- `--json`: Output session info as JSON

Examples:
```bash
shelli create myshell                        # default shell
shelli create pyrepl --cmd "python3"         # Python REPL
shelli create node --cmd "node"              # Node.js REPL
shelli create db --cmd "psql -d mydb"        # PostgreSQL
shelli create server --cmd "ssh user@host"   # SSH session
shelli create redis --cmd "redis-cli"        # Redis CLI
shelli create dev --env "DEBUG=1" --cwd /app # with env and working dir
shelli create wide --cols 200 --rows 50      # large terminal
```

### exec - Send command and wait for result (primary command for AI)

```bash
shelli exec <name> <input> [flags]
```

Sends a command as literal text with newline appended, waits for output to settle or pattern match.

Input is sent exactly as provided - escape sequences like `\n` are NOT interpreted by shelli. They're passed to the shell as-is (the shell may interpret them, e.g., `echo -e`).

For precise control over escape sequences or TUI apps, use `send` instead.

Flags:
- `--settle N`: Wait for N ms of silence (default: 500)
- `--wait "pattern"`: Wait for regex pattern match (mutually exclusive with --settle)
- `--timeout N`: Max wait time in seconds (default: 10)
- `--strip-ansi`: Remove terminal escape codes from output
- `--json`: Output as JSON with input, output, position fields

Examples:
```bash
# Basic execution (waits 500ms for output to settle)
shelli exec pyrepl "print('hello')"

# Longer settle for slow commands
shelli exec db "SELECT * FROM large_table;" --settle 2000

# Wait for specific prompt pattern
shelli exec myshell "ls" --wait '\$\s*$'
shelli exec pyrepl "x = 1" --wait '>>>'

# Clean output for parsing
shelli exec session "command" --strip-ansi --json

# Long-running with timeout
shelli exec session "slow_command" --settle 5000 --timeout 120

# Escape sequences passed to shell (shell interprets them)
shelli exec myshell "echo -e 'hello\nworld'"
```

### send - Send raw input without waiting

```bash
shelli send <name> <input> [input...]
```

Low-level command for precise control:
- Each argument is sent as a separate write to PTY
- Escape sequences are always interpreted
- No newline added automatically

Use `send` for:
- Sending control characters (Ctrl+C, Ctrl+D)
- Answering prompts without newlines
- TUI apps that need separate input chunks
- Fine-grained control over input timing

Examples:
```bash
shelli send myshell "ls -la\n"          # command with explicit newline
shelli send pyrepl "print('hi')\n"      # to Python with newline
shelli send myshell "\x03"              # Ctrl+C (interrupt)
shelli send myshell "\x04"              # Ctrl+D (EOF)
shelli send myshell "y"                 # 'y' without newline
shelli send tui "hello" "\r"            # TUI: type "hello", then Enter (separate writes)
```

### read - Read session output

```bash
shelli read <name> [flags]
```

**Instant modes** (non-blocking):
- (default): New output since last read
- `--all`: All output from session start

**Streaming mode** (for TUIs):
- `--follow` / `-f`: Continuous output like `tail -f`
- `--follow-ms N`: Poll interval in ms (default: 100)

**Blocking modes**:
- `--wait "pattern"`: Wait for regex pattern match
- `--settle N`: Wait for N ms of silence

Other flags:
- `--timeout N`: Max wait time (default: 10s)
- `--strip-ansi`: Remove ANSI escape codes
- `--json`: Output as JSON

Examples:
```bash
shelli read myshell                    # new output, instant
shelli read myshell --all              # all output, instant
shelli read myshell --follow           # stream continuously (Ctrl+C to stop)
shelli read pyrepl --wait ">>>"        # wait for Python prompt
shelli read myshell --settle 300       # wait for 300ms silence
shelli read myshell --strip-ansi       # clean output
```

### list - List all sessions

```bash
shelli list [--json]
```

Shows name, PID, command, created time, running status.

### info - Get detailed session info

```bash
shelli info <name> [--json]
```

Shows detailed session information: name, state, pid, command, created_at, stopped_at (if stopped), uptime, buffer size, read position, terminal dimensions.

### clear - Clear output buffer

```bash
shelli clear <name> [--json]
```

Truncates the output buffer and resets the read position. The session continues running.

### resize - Change terminal dimensions

```bash
shelli resize <name> [--cols N] [--rows N] [--json]
```

At least one of `--cols` or `--rows` must be specified. Omitted dimensions keep their current value.

Examples:
```bash
shelli resize myshell --cols 120 --rows 40   # set both
shelli resize myshell --cols 200             # change only width
```

### stop - Stop session (keep output)

```bash
shelli stop <name> [--json]
```

Terminates the process but keeps output readable. Session stays in list with state "stopped".

### kill - Kill a session

```bash
shelli kill <name> [--json]
```

Terminates the session and cleans up all resources (output and metadata).

## Escape Sequences (for send --raw)

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
| `\x03` | Ctrl+C | Interrupt (SIGINT) - stop running command |
| `\x04` | Ctrl+D | EOF - close stdin, often exits REPL |
| `\x1a` | Ctrl+Z | Suspend (SIGTSTP) |
| `\x1c` | Ctrl+\ | Quit (SIGQUIT) |
| `\x0c` | Ctrl+L | Clear screen |
| `\x09` | Tab | Tab character (completion) |
| `\x7f` | Backspace | Delete previous character |
| `\x15` | Ctrl+U | Kill line (clear current input) |
| `\x17` | Ctrl+W | Kill word (delete previous word) |

## Handling Complex Input (MCP)

When using MCP tools via `mcp-cli`, input goes through two escaping layers:
1. **Shell escaping** - for the bash command itself
2. **JSON escaping** - for the JSON string value

### JSON Escaping Rules

In JSON strings, these characters MUST be escaped:
- `"` → `\"`
- `\` → `\\` (CRITICAL: bare backslashes cause "Invalid escape character" errors)
- newline → `\n`
- tab → `\t`

### Recommended: Use stdin mode (heredoc)

For complex input, use heredoc with stdin mode to avoid the shell escaping layer:

```bash
mcp-cli call shelli/send - <<'EOF'
{
  "name": "session",
  "input": "print(\"Hello\\nWorld\")"
}
EOF
```

Note: Only JSON escaping needed inside heredoc - no shell escaping layer.

### Common Pitfalls

- `\!`, `\$`, `\@` are NOT valid JSON escapes → use `\\!`, `\\$`, `\\@` or just `!`, `$`, `@`
- Single quotes in bash don't prevent JSON escaping requirements
- Nested quotes need double escaping: `"say \"hi\""`

### Examples

Simple command (inline JSON works fine):
```bash
mcp-cli call shelli/send '{"name": "sh", "input": "ls -la"}'
```

Python with quotes (use heredoc):
```bash
mcp-cli call shelli/send - <<'EOF'
{"name": "py", "input": "print(\"hello\")"}
EOF
```

SQL with quotes (use heredoc):
```bash
mcp-cli call shelli/send - <<'EOF'
{"name": "db", "input": "SELECT * FROM users WHERE name = 'O''Brien'"}
EOF
```

### Fallback: Base64 Encoding

When escaping becomes unmanageable (deeply nested quotes, binary data), use `input_base64`:

```bash
# Original: print("Hello\nWorld")
# Base64: cHJpbnQoIkhlbGxvXG5Xb3JsZCIp
mcp-cli call shelli/send '{"name": "py", "input_base64": "cHJpbnQoIkhlbGxvXG5Xb3JsZCIp"}'
```

Trade-off: 33% larger payload, but eliminates all escaping complexity.

Note: `input` and `input_base64` are mutually exclusive - use one or the other.

## Multi-Input: Use `inputs` Array for Sequences

When sending content followed by Enter/Return or any multi-step input, **always use the `inputs` array** in a single MCP call. This is more efficient than multiple separate calls.

### When to Use `inputs`

```json
// CORRECT - single call, two PTY writes
{"name": "session", "inputs": ["Hello Zephyr!", "\r"]}

// WRONG - wastes tokens on two calls
{"name": "session", "input": "Hello Zephyr!"}  // first call
{"name": "session", "input": "\r"}              // second call
```

### Common Patterns

| Scenario | MCP JSON |
|----------|----------|
| Message + Enter | `{"inputs": ["your message", "\r"]}` |
| Command + confirmation | `{"inputs": ["rm -i file.txt", "\r", "y", "\r"]}` |
| Multi-line input | `{"inputs": ["line1", "\r", "line2", "\r"]}` |
| Password entry | `{"inputs": ["password123", "\r"]}` |

### Why This Matters

- **Efficiency**: One API call instead of two (or more)
- **Correctness**: TUI apps often need Enter as a separate PTY write event
- **Token savings**: Less overhead per interaction

### CLI Equivalent

The CLI already supports this pattern with multiple arguments:

```bash
shelli send session "message" "\r"
```

This sends `"message"` and `"\r"` as separate writes, matching the MCP `inputs` behavior.

## Best Practices

### Settle Times

Default settle time is 500ms. Adjust based on expected response time:

| Scenario | Recommended Settle |
|----------|-------------------|
| Simple REPL commands | 300-500ms |
| File operations | 500-1000ms |
| Network operations | 1000-2000ms |
| Database queries | 1000-3000ms |
| Remote SSH commands | 2000-5000ms |

### Wait Patterns

Use `--wait` instead of `--settle` when you know the expected output pattern:

```bash
# Shell prompts
--wait '\$\s*$'           # bash prompt ending with $
--wait '#\s*$'            # root prompt ending with #
--wait '>\s*$'            # generic prompt ending with >

# REPL prompts
--wait '>>>\s*$'          # Python
--wait '>\s*$'            # Node.js
--wait 'irb.*>\s*$'       # Ruby IRB
--wait 'iex.*>\s*$'       # Elixir

# Database prompts
--wait '=>\s*$'           # psql
--wait 'mysql>\s*$'       # MySQL
--wait 'sqlite>\s*$'      # SQLite

# Custom patterns
--wait 'Password:'        # Password prompt
--wait '\[y/n\]'          # Yes/no prompt
--wait 'Enter.*:'         # Generic input prompt
```

### Session Naming

Use descriptive, collision-free names:

```bash
# Good
shelli create python-data-analysis --cmd "python3"
shelli create ssh-prod-server --cmd "ssh user@prod.example.com"
shelli create postgres-mydb --cmd "psql -d mydb"

# Avoid
shelli create s1 --cmd "python3"  # Not descriptive
shelli create test --cmd "ssh"    # Too generic
```

### Session Lifecycle

1. **Create when needed**: Only create sessions when you actually need persistence
2. **Reuse existing sessions**: Check `shelli list` before creating duplicates
3. **Clean up when done**: Kill sessions when the workflow is complete
4. **Handle stuck sessions**: Use Ctrl+C (`\x03`) to interrupt, Ctrl+D (`\x04`) to exit

## Common Workflow Patterns

### Python REPL

```bash
# Create and wait for prompt
shelli create py --cmd "python3"
shelli read py --wait '>>>'

# Execute commands
shelli exec py "import pandas as pd" --wait '>>>'
shelli exec py "df = pd.read_csv('data.csv')" --wait '>>>'
shelli exec py "df.describe()" --strip-ansi

# Clean up
shelli kill py
```

### SSH Session

```bash
# Create SSH connection
shelli create remote --cmd "ssh user@server.example.com"
shelli read remote --wait '\$\s*$' --timeout 30  # wait for login

# Run commands
shelli exec remote "cd /var/log" --wait '\$'
shelli exec remote "tail -n 50 app.log" --strip-ansi
shelli exec remote "grep ERROR app.log | wc -l" --strip-ansi

# Clean up
shelli exec remote "exit"
shelli kill remote
```

### Database CLI

```bash
# Connect to PostgreSQL
shelli create db --cmd "psql -h localhost -U myuser -d mydb"
shelli read db --wait '=>\s*$' --timeout 10

# Run queries
shelli exec db "SELECT count(*) FROM users;" --wait '=>' --strip-ansi
shelli exec db "\\dt" --wait '=>' --strip-ansi  # list tables

# Transaction workflow
shelli exec db "BEGIN;" --wait '=>'
shelli exec db "UPDATE users SET active = true WHERE id = 1;" --wait '=>'
shelli exec db "COMMIT;" --wait '=>'

# Clean up
shelli exec db "\\q"
shelli kill db
```

### Interactive Prompt Handling

```bash
# When a command asks for confirmation
shelli send session "rm -i file.txt"
shelli read session --wait '\[y/n\]'
shelli send session "y"  # Answer with newline
shelli read session --settle 500

# Password prompt (be careful with credentials)
shelli send session "sudo command"
shelli read session --wait 'Password:'
shelli send session "password"  # Sends with newline
shelli read session --settle 1000
```

## Error Handling

### Timeout Errors

If commands timeout:
1. Increase `--timeout` value
2. Check if the session is still running (`shelli list`)
3. Try reading current output (`shelli read <name> --all`)
4. Send Ctrl+C to interrupt (`shelli send <name> "\x03" --raw`)

### Stuck Sessions

```bash
# Interrupt current command
shelli send session "\x03" --raw
shelli read session --settle 500

# If still stuck, send EOF
shelli send session "\x04" --raw

# Force kill as last resort
shelli kill session
```

### Session Not Found

```bash
# Check if session exists
shelli list

# Recreate if needed
shelli create session --cmd "command"
```

## Architecture Notes

- **Daemon-based**: First command auto-starts daemon if not running
- **PTY-backed**: Sessions use pseudo-terminals for full terminal emulation
- **Output buffering**: All output is buffered with position tracking
- **Socket communication**: CLI talks to daemon via Unix socket (`~/.shelli/shelli.sock`)
- **Max output**: Default 10MB buffer per session (configurable via daemon `--max-output`)

## Limitations

### TUI Applications - Now Supported

shelli supports TUI applications using `--follow` mode for continuous streaming:

```bash
shelli create mon --cmd "btop"
shelli read mon --follow              # streams output, renders TUI
shelli resize mon --cols 150 --rows 50  # resize works too
```

**What works well:**
- System monitors: `btop`, `htop`, `k9s`
- Dashboards and status displays
- Most TUIs using standard terminal escape sequences

**Limited support:**
- Text editors (`vim`, `nano`) - display works but interaction is impractical
- Apps with complex mouse handling may behave unexpectedly
- Some apps may not respond to resize signals

### Line-Based TUI Applications

Some TUI apps use line-based input/output and work with shelli, but may need special handling:

**Example: OpenClaw TUI**

OpenClaw TUI (`openclaw tui`) is a chat interface for AI agents. It works with shelli by sending message and Enter as separate writes:

```bash
# Step 1: Create SSH session and launch TUI
shelli create openclaw --cmd "ssh user@host"
shelli read openclaw --settle 3000
shelli send openclaw "openclaw tui\n"
shelli read openclaw --settle 3000

# Step 2: Send message then Enter as separate writes
shelli send openclaw "Hello, this is my message" "\r"

# Step 3: Wait for response
sleep 8  # Allow time for AI to respond
shelli read openclaw --strip-ansi
```

**Why separate writes?**

TUI apps often buffer input and only submit when Enter is pressed as a separate keypress event. By using multiple arguments:
1. First arg sends your text
2. Second arg `"\r"` sends carriage return as a separate write, triggering submit

**Pattern for TUIs with input buffers:**
```bash
# Message then Enter as separate writes
shelli send session "your message" "\r"

# If \r doesn't work, try \n
shelli send session "your message" "\n"
```

**Debugging TUI issues:**
1. Read all output: `shelli read session --all --strip-ansi`
2. Look for status lines showing "idle" or "connected"
3. Check if your message appears in input area vs chat history
4. If stuck, try Ctrl+C: `shelli send session "\x03"`
