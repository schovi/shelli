# Release Notes Skill

Write GitHub release notes for shelli releases with consistent structure and detail level.

## When to Use

After creating a git tag and the goreleaser workflow completes, update the release notes using:

```bash
gh release edit <tag> --notes "$(cat <<'EOF'
<release notes here>
EOF
)"
```

## Release Notes Structure

### 1. Breaking Changes (if any)

Start with `## Breaking Changes` section when the release includes breaking changes (indicated by `!` in commit message or semver minor bump pre-1.0).

For each breaking change:

```markdown
## Breaking Changes

### <Component> <brief description of change>

<1-2 sentence summary of what changed and why>

- **Change 1**: Description
- **Change 2**: Description
- **Change 3**: Description
```

### 2. Comparison Table (when clarifying roles/behavior)

When changes affect how components relate to each other, include a comparison table:

```markdown
### `exec` vs `send` - Clear Roles

| Command | Purpose | Newline | Escapes | Wait |
|---------|---------|---------|---------|------|
| `exec`  | Run commands | Auto-added | NOT interpreted | Yes |
| `send`  | Send raw bytes | Manual | Always interpreted | No |
```

### 3. Migration Examples (for breaking changes)

Always include Before/After examples when behavior changes:

```markdown
### Migration

**If you were using <old pattern>:**
```bash
# Before (v0.X.0)
<old command>

# After (v0.Y.0)
<new command>
```

**<Another scenario>:**
```bash
# Explanation of what happens
<command example>
```
```

### 4. New Features (if any)

```markdown
## New Features

### <Feature name>

<Description of the feature>

```bash
# Example usage
<command>
```
```

### 5. Bug Fixes (if any)

```markdown
## Bug Fixes

- Fixed <issue description>
- Fixed <issue description>
```

### 6. Documentation / Other Changes

End with minor changes that don't fit above:

```markdown
## Documentation

- Updated CLI help text to clarify X
- Updated README with Y
- Added examples for Z
```

## Level of Detail

- **Be specific**: Name exact flags, parameters, or behaviors that changed
- **Show code**: Always include command examples for anything behavioral
- **Explain why**: Brief context for breaking changes helps users understand
- **Migration path**: Never leave users guessing how to update their usage

## Example: Complete Release Notes

```markdown
## Breaking Changes

### `send` command simplified to low-level raw interface

The `send` command has been redesigned to be a low-level, precise control interface:

- **Removed flags**: `--raw`, `--multi`, `--submit` are all gone
- **Always interprets escapes**: `\n`, `\r`, `\x03`, etc. are always interpreted
- **No auto-newline**: You must explicitly add `\n` or `\r` when needed

### `exec` vs `send` - Clear Roles

| Command | Purpose | Newline | Escapes | Wait |
|---------|---------|---------|---------|------|
| `exec`  | Run commands | Auto-added | NOT interpreted | Yes |
| `send`  | Send raw bytes | Manual | Always interpreted | No |

### Migration

**Before:**
```bash
shelli send session "ls"              # sent with newline
shelli send session "hello" --raw     # sent without newline
```

**After:**
```bash
shelli send session "ls\n"            # explicit newline
shelli send session "hello"           # no newline
```

## Documentation

- Updated CLI help text to clarify escape sequence behavior
- Updated README with clearer exec vs send guidance
```

## Checklist Before Publishing

1. [ ] Breaking changes clearly marked with `## Breaking Changes`
2. [ ] Each breaking change has migration examples
3. [ ] Command examples use actual shelli syntax
4. [ ] Tables used for comparisons (when applicable)
5. [ ] Non-breaking changes in separate section at end
