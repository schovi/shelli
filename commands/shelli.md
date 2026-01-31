# /shelli - Force Persistent Session Usage

<command-name>shelli</command-name>

Use this command to explicitly request shelli for a task. This overrides automatic detection and ensures a persistent interactive session is used.

## Arguments

The argument is a natural language instruction describing what you want to do:

```
/shelli <instruction>
```

## Examples

```
/shelli connect to production and check disk space
/shelli start a Python session for data analysis
/shelli open PostgreSQL and explore the schema
/shelli run the setup wizard interactively
/shelli start the dev server and watch for errors
```

## Behavior

When invoked:

1. Parse the instruction to determine the appropriate command
2. Create a named session based on the task
3. Execute the workflow using shelli
4. Maintain the session for follow-up interactions
5. Clean up when explicitly requested or when done

## When to Use

The auto-detector handles most cases automatically. Use `/shelli` when:

- You want to force session usage for a task that might not auto-detect
- You want explicit control over session creation
- You're starting a complex workflow that you know needs persistence

## Session Management

After `/shelli` creates a session, you can:

- Continue with follow-up requests (session stays open)
- Ask to "kill the session" or "close it" when done
- Check sessions with "list shelli sessions"
