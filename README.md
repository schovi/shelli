# ishell

Interactive shell session manager for AI agents. Enables persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.) that survive across CLI invocations.

## Installation

```bash
go install github.com/schovi/ishell@latest
```

Or build from source:
```bash
git clone https://github.com/schovi/ishell
cd ishell
go build -o ishell .
```

## Commands

### create

Create a new interactive session.

```bash
ishell create <name> [--cmd "command"] [--json]
```

Examples:
```bash
ishell create myshell                        # default shell
ishell create pyrepl --cmd "python3"         # Python REPL
ishell create db --cmd "psql -d mydb"        # PostgreSQL
ishell create server --cmd "ssh user@host"   # SSH session
```

### send

Send input to a session. Appends newline by default.

```bash
ishell send <name> <input> [--raw]
```

**Normal mode** (default): Appends newline, sends as-is.

**Raw mode** (`--raw`): No newline, interprets escape sequences.

Examples:
```bash
ishell send myshell "ls -la"           # send command + newline
ishell send pyrepl "print('hello')"    # send to Python + newline
ishell send myshell "\x03" --raw       # send Ctrl+C
ishell send myshell "\x04" --raw       # send Ctrl+D (EOF)
ishell send myshell "y" --raw          # send 'y' without newline
```

### exec

Send input and wait for result. The primary command for AI agents.

```bash
ishell exec <name> <input> [flags]
```

Flags:
- `--settle N` - Wait for N ms of silence (default: 500)
- `--wait "pattern"` - Wait for regex pattern match (mutually exclusive with --settle)
- `--timeout N` - Max wait time in seconds (default: 10)
- `--strip-ansi` - Remove terminal escape codes
- `--json` - Output as JSON

Examples:
```bash
ishell exec pyrepl "print('hello')"                # wait for output to settle
ishell exec pyrepl "print('hello')" --settle 1000  # longer settle
ishell exec myshell "ls" --wait '\$'               # wait for shell prompt
ishell exec db "SELECT 1;" --strip-ansi --json     # clean JSON output
```

### read

Read output from a session.

```bash
ishell read <name> [flags]
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
ishell read myshell                    # new output, instant
ishell read myshell --all              # all output, instant
ishell read pyrepl --wait ">>>"        # wait for Python prompt
ishell read myshell --settle 300       # wait for 300ms silence
ishell read myshell --strip-ansi       # clean output
```

### list

List all sessions.

```bash
ishell list [--json]
```

### kill

Kill a session.

```bash
ishell kill <name>
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
ishell send myshell "\x03" --raw

# Send EOF to close stdin
ishell send myshell "\x04" --raw

# Suspend a process
ishell send myshell "\x1a" --raw

# Tab completion
ishell send myshell "doc\t" --raw

# Answer a yes/no prompt without newline, then send newline
ishell send myshell "y" --raw
ishell send myshell ""              # just newline

# Send escape sequence (e.g., for terminal commands)
ishell send myshell "\e[2J" --raw   # clear screen (ANSI)
```

## Architecture

ishell uses a daemon process to maintain PTY handles across CLI invocations:

- First command auto-starts the daemon if not running
- Daemon holds all PTY handles in memory
- CLI commands communicate via Unix socket (`~/.ishell/ishell.sock`)
- Output is buffered with read position tracking

## For AI Agents

The `exec` command is designed for AI agent workflows:

```bash
# Simple command execution
ishell exec session "ls -la" --strip-ansi

# With JSON for parsing
ishell exec session "echo hello" --json
# {"input":"echo hello","output":"hello\n","position":123}

# Custom settle time for slow commands
ishell exec session "slow_command" --settle 2000 --timeout 60

# Interrupt a stuck command
ishell send session "\x03" --raw
```

Typical agent workflow:
```bash
ishell create session --cmd "python3"
ishell exec session "x = 42"
ishell exec session "print(x * 2)" --strip-ansi
# Output: 84
ishell kill session
```

## Version

v0.2.1
