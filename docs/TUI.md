# TUI Mode

TUI mode enables shelli to work with full-screen terminal applications (htop, vim, lazygit, etc.) using a proper VT terminal emulator.

## Overview

Without TUI mode, shelli's output buffer grows indefinitely as TUI apps repaint the screen. Each repaint appends new escape sequences and content, making the buffer unreadable and eventually hitting memory limits.

TUI mode solves this by replacing raw byte storage with a VT terminal emulator (`charmbracelet/x/vt`). The emulator IS the screen state, so reads always return the current screen content regardless of how many repaints have occurred.

Enable with `--tui` on session creation:
```bash
shelli create myapp --cmd htop --tui
```

## Architecture

### How it works

Each TUI session gets a `vterm.Screen` wrapper around a thread-safe VT emulator:

```
PTY output → screen.Write() (feeds VT emulator)
             screen.Render() → ANSI-styled screen content (for reads)
             screen.String() → plain text screen content (for snapshots/strip)
             screen.ReadResponses(ptmx) → bridges terminal query responses to PTY
```

No raw byte storage is used for TUI sessions. The emulator handles:
- Cursor positioning (absolute, relative, home)
- Screen clearing (ESC[2J, ESC[J, etc.)
- Line erasing (ESC[K, etc.)
- Alt screen buffer (ESC[?1049h/l)
- Synchronized updates (ESC[?2026h/l)
- DEC Special Graphics charset
- Color and text attributes (SGR)
- Terminal capability queries (DA1, DA2, DSR, etc.)

### Version counter

An atomic version counter increments on every `Write()`. This replaces byte-count-based change detection:
- `handleSize` returns the version counter for TUI sessions
- `handleRead` with `ReadModeNew` compares version against stored read position
- Wait/settle loops poll the version counter

### Non-TUI sessions

Non-TUI sessions are unchanged: raw byte storage with the existing OutputStorage interface.

## Terminal Query Responses

The VT emulator handles terminal capability queries internally. When an app sends a query (e.g., DA1 `ESC[c`), the emulator generates a response and writes it to an internal pipe. A `ReadResponses` goroutine reads from this pipe and writes to the PTY master, appearing as terminal input to the subprocess.

Handled queries include:
- DA1 (Primary Device Attributes): `ESC[c` / `ESC[0c`
- DA2 (Secondary Device Attributes): `ESC[>c` / `ESC[>0c`
- DSR (Device Status Report): `ESC[5n`, `ESC[6n`
- Cursor Position Report

This replaces the old hand-rolled `TerminalResponder` with the emulator's built-in handlers.

## Snapshot Mechanism

Snapshot (`--snapshot` on read) provides a clean, current frame by forcing a full redraw.

### Flow

1. **Cold start wait**: If `screen.Version() == 0`, wait up to 2s for initial content
2. **Resize cycle**: Set terminal to (cols+1, rows+1) and resize emulator to match, send SIGWINCH, pause 200ms, restore original size, send SIGWINCH
3. **Settle loop**: Poll `screen.Version()` every 25ms until stable for `settle_ms` (default 300ms)
4. **Retry**: If output is still empty, send another SIGWINCH with 2x settle time
5. Return `screen.String()` (plain text)

### Why resize?

TUI apps listen for SIGWINCH (window size change) and perform a full redraw. The emulator is also resized to match, so it correctly interprets the redrawn content at the right dimensions.

## ANSI Stripping

The `vterm.Strip()` function (`internal/vterm/strip.go`) removes ANSI escape sequences from text.

### Two paths

1. **Fast path (no cursor sequences)**: Regex-based stripping of CSI, OSC, charset, keypad, DEC private mode, and ESC+letter sequences.

2. **Emulator path (has cursor sequences)**: Creates a temporary VT emulator, writes the content, reads back `String()` (plain text). Handles all cursor positioning, erasing, and character rendering correctly.

Detection: checks for cursor positioning patterns (`ESC[n;nH`, `ESC[nG`, `ESC[nd`, `ESC[nA/B/C/D`).

Standalone `\n` (not preceded by `\r`) is converted to `\r\n` before feeding to the emulator, matching what a real terminal driver does with ONLCR.

## App Compatibility

| App | Snapshot | Strip-ANSI | Notes |
|-----|----------|------------|-------|
| btop | Clean | Good | HVP cursor (CSI f) support |
| htop | Clean | Good | Reference-quality TUI support |
| glances | Clean | Good | Needs --settle 3000 for consistency |
| k9s | Clean | Partial | Left columns truncated in strip-ansi |
| ranger | Clean | Good | |
| nnn | Clean | Good | |
| yazi | Clean | Partial | Only right pane in strip-ansi |
| vifm | Clean | Good | |
| lazygit | Clean | Good | |
| tig | Clean | Good | |
| vim | Clean | Good | |
| less | Clean | Good | |
| micro | Clean | Good | |
| weechat | Clean | Good | Timing-dependent at large sizes |
| irssi | Clean | Good | |
| newsboat | Clean | Good | |
| mc | Clean | Good | Heavy box drawing renders well |
| bat | Clean | Good | |
| ncdu | Clean | Good | |

## Known Limitations

### Direct /dev/tty Access

Some apps open `/dev/tty` directly instead of using stdin/stdout. These bypass the PTY entirely, making shelli unable to capture their output or respond to terminal queries. Most apps work fine, but this is a known edge case.

### Wide Characters

CJK characters and some emoji occupy two terminal cells. The VT emulator handles display width correctly via the `displaywidth` package, but some edge cases with complex grapheme clusters may still cause alignment issues.

### Partial Strip-ANSI for Complex Layouts

Some apps with complex multi-pane layouts produce partial strip-ansi output:
- **k9s**: Left-side columns may be truncated in strip-ansi snapshots
- **yazi**: Only the right pane renders in strip-ansi (raw snapshots are complete)

## Constants Reference

| Constant | Value | Location | Purpose |
|----------|-------|----------|---------|
| `DefaultSnapshotSettleMs` | 300ms | `constants.go` | Default settle time for snapshot |
| `SnapshotPollInterval` | 25ms | `constants.go` | Polling interval during snapshot settle |
| `SnapshotResizePause` | 200ms | `constants.go` | Pause between resize steps |
