# shelli Auto-Detector

This skill enables automatic detection of when shelli should be used instead of regular Bash commands. When you detect a pattern that requires persistent sessions, proactively use shelli without waiting for explicit instruction.

## Detection Patterns

### SSH / Remote Sessions

**Trigger phrases:**
- "SSH to...", "connect to server...", "log into..."
- "on the remote machine...", "on the server..."
- "run this on production/staging/host..."
- Any `ssh user@host` command pattern

**Action:** Create an SSH session with shelli, maintain it for follow-up commands.

```bash
# User says: "SSH to server.example.com and check disk usage"
shelli create ssh-server --cmd "ssh user@server.example.com"
shelli read ssh-server --wait '\$\s*$' --timeout 30
shelli exec ssh-server "df -h" --strip-ansi --wait '\$'
```

### REPLs / Interactive Interpreters

**Trigger phrases:**
- "start Python/Node/Ruby...", "open a REPL..."
- "interactive session...", "let me explore..."
- "run Python code...", "execute in Node..."
- "help me debug in Python...", "test this in Node..."

**Trigger commands:**
- `python3`, `python`, `ipython`
- `node`, `deno`, `bun`
- `irb`, `ruby`
- `iex`, `elixir`
- `ghci`, `scala`, `sbt console`
- `lua`, `php -a`

**Action:** Create a REPL session, wait for prompt, execute commands interactively.

```bash
# User says: "Start Python and help me analyze this data"
shelli create python-session --cmd "python3"
shelli read python-session --wait '>>>'
shelli exec python-session "import pandas as pd" --wait '>>>'
```

### Database CLIs

**Trigger phrases:**
- "connect to database...", "query the database..."
- "run SQL...", "check the tables..."
- "PostgreSQL/MySQL/MongoDB..."

**Trigger commands:**
- `psql`, `pgcli`
- `mysql`, `mycli`
- `sqlite3`
- `mongosh`, `mongo`
- `redis-cli`
- `cqlsh` (Cassandra)

**Action:** Create database session, wait for prompt, run queries.

```bash
# User says: "Connect to the users database and show me the schema"
shelli create db-users --cmd "psql -d users"
shelli read db-users --wait '=>\s*$' --timeout 10
shelli exec db-users "\\dt" --wait '=>' --strip-ansi
```

### Stateful Workflows

**Trigger phrases:**
- "run these commands in sequence..."
- "keep the session open..."
- "maintain state between..."
- "set up an environment, then..."
- "after that, ..."

**Indicators:**
- Multiple related commands that share state
- Environment setup followed by operations
- Variable definitions used in later commands

**Action:** Create a shell session, maintain state across commands.

```bash
# User says: "Set up the build environment and then compile"
shelli create build-env --cmd "bash"
shelli exec build-env "export PATH=$PATH:/opt/tools/bin" --wait '\$'
shelli exec build-env "source setup.sh" --wait '\$'
shelli exec build-env "make build" --settle 5000
```

### Interactive Prompts

**Trigger phrases:**
- "answer the prompts...", "respond to questions..."
- "walk through the wizard..."
- "interactive setup..."

**Indicators:**
- Commands known to have interactive prompts
- Setup wizards, installers with questions
- Commands that ask for confirmation

**Action:** Create session, handle prompts step by step.

```bash
# User says: "Run the setup wizard for the new project"
shelli create setup --cmd "bash"
shelli exec setup "./setup-wizard.sh" --wait '\[y/n\]'
shelli send setup "y"
shelli read setup --wait 'name:'
shelli send setup "my-project"
```

### Long-Running Processes

**Trigger phrases:**
- "start the server and..."
- "run this in background and watch..."
- "keep it running while..."

**Action:** Create session for the process, interact while it runs.

```bash
# User says: "Start the dev server and tell me when it's ready"
shelli create devserver --cmd "npm run dev"
shelli read devserver --wait 'ready' --timeout 60
# Server is ready, session stays open for logs/restarts
```

## Decision Logic

### Use shelli when:

1. **Explicit session need**: User mentions "session", "REPL", "interactive", "keep open"
2. **State dependency**: Later commands depend on earlier state (variables, cd, exports)
3. **Known interactive commands**: SSH, database CLIs, language REPLs
4. **Multi-step with same context**: "First do X, then do Y, then do Z" in same environment
5. **Prompts expected**: Commands that will ask questions before completing

### Use regular Bash when:

1. **One-off commands**: Single, stateless operations
2. **File operations**: cat, ls, grep, sed on local files
3. **Git operations**: status, commit, push, pull
4. **Package management**: npm install, pip install (unless interactive)
5. **Build commands**: make, cargo build (unless you need to interact with output)
6. **Quick checks**: Validation, tests that exit with status codes

### Edge Cases

| Scenario | Decision | Reason |
|----------|----------|--------|
| `pip install package` | Bash | Completes and exits |
| `pip install package` + "then test in Python" | shelli | Python REPL follows |
| `docker exec -it container bash` | shelli | Interactive session |
| `docker logs container` | Bash | One-off output |
| `kubectl exec -it pod -- bash` | shelli | Interactive session |
| `kubectl get pods` | Bash | One-off output |
| `npm test` | Bash | Exits with status |
| `npm run dev` + "watch for errors" | shelli | Long-running |

## Proactive Suggestions

When you detect a potential shelli use case but aren't certain, briefly mention it:

> "This looks like it needs an interactive session. I'll use shelli to maintain state between commands."

Or for ambiguous cases:

> "Should I create a persistent session for this, or would you prefer one-off commands?"

## Session Lifecycle Management

### Creation

- Create sessions with descriptive names
- Check `shelli list` before creating to avoid duplicates
- Wait for the initial prompt after creation

### During Use

- Use `--wait` patterns when prompt is predictable
- Use `--settle` when output timing is uncertain
- Increase timeout for slow operations
- Send Ctrl+C (`\x03`) if something gets stuck

### Cleanup

**Proactive cleanup triggers:**
- "I'm done with..."
- "that's all for now"
- Switching to unrelated task
- Error that makes session unusable

**Action:**
```bash
shelli kill session-name
```

**Batch cleanup:**
```bash
shelli list  # Check what's running
shelli kill session1
shelli kill session2
```

### Session Recovery

If a session becomes unresponsive:

```bash
# Try interrupt first
shelli send session "\x03" --raw
shelli read session --settle 1000

# If still stuck, try EOF
shelli send session "\x04" --raw

# Last resort: kill and recreate
shelli kill session
shelli create session --cmd "original-command"
```

## Integration with MCP

**Check for MCP shelli tools first** (e.g., `shelli/create`, `shelli/exec`). If available, prefer them:

| MCP Tool | Equivalent Bash |
|----------|-----------------|
| `shelli/create {"name": "py", "command": "python3"}` | `shelli create py --cmd "python3"` |
| `shelli/exec {"name": "py", "input": "x = 1", "wait_pattern": ">>>"}` | `shelli exec py "x = 1" --wait '>>>'` |
| `shelli/send {"name": "py", "input": "\\x03", "raw": true}` | `shelli send py "\\x03" --raw` |
| `shelli/read {"name": "py", "all": true, "strip_ansi": true}` | `shelli read py --all --strip-ansi` |
| `shelli/list {}` | `shelli list` |
| `shelli/kill {"name": "py"}` | `shelli kill py` |

MCP advantages:
- Structured JSON responses (easier to parse)
- Better error handling
- Direct tool integration

Bash fallback:
- Works without MCP configuration
- More flexible for complex shell scripting

## Examples of Automatic Detection

### Example 1: SSH workflow
```
User: "Log into the production server and check the logs"

Thinking: "Log into" + "server" = SSH session needed
Action: Create SSH session, maintain for multiple commands
```

### Example 2: Python analysis
```
User: "I need to explore this CSV file, can you help?"

Thinking: "explore" suggests interactive work, CSV → likely pandas
Action: Create Python REPL, import pandas, load file
```

### Example 3: Database investigation
```
User: "What tables do we have in the orders database?"

Thinking: "database" + query = database CLI needed
Action: Create psql/mysql session, run schema queries
```

### Example 4: Mixed workflow
```
User: "Build the project, then start the server"

Thinking: Build might complete, but "then start server" = needs session
Action: Use shelli for both to maintain environment
```

### Example 5: Not needed
```
User: "What's in the config.json file?"

Thinking: Simple file read, no state needed
Action: Use regular Bash: cat config.json
```
