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

## Usage

### Create a session

```bash
# Default shell
ishell create myshell

# Specific command
ishell create pyrepl --cmd "python3"
ishell create psql --cmd "psql -d mydb"
```

### Send input

```bash
# Send without newline
ishell send myshell "partial input"

# Send with newline (execute command)
ishell sendline pyrepl 'print("hello")'
```

### Read output

```bash
# Read new output since last read
ishell read myshell

# Read all output
ishell read myshell --mode all

# JSON output for programmatic use
ishell read myshell --json
```

### Wait for pattern

```bash
# Wait for Python prompt
ishell wait pyrepl ">>>"

# Wait with timeout
ishell wait myshell "completed" --timeout 60
```

### List sessions

```bash
ishell list
ishell list --json
```

### Kill a session

```bash
ishell kill myshell
```

## Architecture

ishell uses a daemon process to maintain PTY handles across CLI invocations:

- First command auto-starts the daemon if not running
- Daemon holds all PTY handles in memory
- CLI commands communicate with daemon via Unix socket (`~/.ishell/ishell.sock`)
- Session output is buffered in memory with read position tracking

## For AI Agents

JSON output mode (`--json`) provides structured output for programmatic use:

```bash
# Create and parse response
ishell create test --cmd "python3" --json
# {"name":"test","pid":12345,"command":"python3","created_at":"..."}

# Read with position tracking
ishell read test --json
# {"output":"...","position":1234}

# Wait with match status
ishell wait test ">>>" --json
# {"matched":true,"output":"...","position":1234}
```

## Version

v0.1.0 - Core functionality
