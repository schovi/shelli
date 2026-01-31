# 🐚 shelli - Persistent shell sessions for AI agents

**Shell Interactive** - session manager for AI agents. Enables persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.) that survive across CLI invocations.

## Installation

```bash
go install github.com/schovi/shelli@latest
```

Or build from source:
```bash
git clone https://github.com/schovi/shelli
cd shelli
go build -o shelli .
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
shelli read myshell --strip-ansi       # clean output
```

### list

List all sessions.

```bash
shelli list [--json]
```

### kill

Kill a session.

```bash
shelli kill <name>
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

# Suspend a process
shelli send myshell "\x1a" --raw

# Tab completion
shelli send myshell "doc\t" --raw

# Answer a yes/no prompt without newline, then send newline
shelli send myshell "y" --raw
shelli send myshell ""              # just newline

# Send escape sequence (e.g., for terminal commands)
shelli send myshell "\e[2J" --raw   # clear screen (ANSI)
```

## Architecture

shelli uses a daemon process to maintain PTY handles across CLI invocations:

- First command auto-starts the daemon if not running
- Daemon holds all PTY handles in memory
- CLI commands communicate via Unix socket (`~/.shelli/shelli.sock`)
- Output is buffered with read position tracking

## For AI Agents

The `exec` command is designed for AI agent workflows:

```bash
# Simple command execution
shelli exec session "ls -la" --strip-ansi

# With JSON for parsing
shelli exec session "echo hello" --json
# {"input":"echo hello","output":"hello\n","position":123}

# Custom settle time for slow commands
shelli exec session "slow_command" --settle 2000 --timeout 60

# Interrupt a stuck command
shelli send session "\x03" --raw
```

Typical agent workflow:
```bash
shelli create session --cmd "python3"
shelli exec session "x = 42"
shelli exec session "print(x * 2)" --strip-ansi
# Output: 84
shelli kill session
```

## Limitations

### TUI Applications

shelli does **not** support full-screen TUI applications like `k9s`, `btop`, `htop`, `vim`, `nano`, etc.

These apps don't produce line-based output - they paint a 2D screen using cursor positioning and ANSI escape sequences. The raw output is screen-drawing instructions, not readable content.

**Workarounds:**
- Use the underlying CLI tools instead:
  - `k9s` → `kubectl get pods`, `kubectl describe pod`
  - `btop`/`htop` → `ps aux`, `top -bn1`
  - `vim` → `sed`, `awk`, or direct file manipulation
- For apps with no CLI alternative, consider using them outside shelli

shelli works best with:
- REPLs (Python, Node, Ruby, etc.)
- Database CLIs (psql, mysql, sqlite3)
- SSH sessions (for running commands on remote hosts)
- Any tool that produces line-based text output

## Version

v0.3.0
