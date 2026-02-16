# TUI Mode

TUI mode enables shelli to work with full-screen terminal applications (htop, vim, lazygit, etc.) by detecting frame boundaries and managing buffer truncation.

## Overview

Without TUI mode, shelli's output buffer grows indefinitely as TUI apps repaint the screen. Each repaint appends new escape sequences and content, making the buffer unreadable and eventually hitting memory limits.

TUI mode solves this by:
1. Detecting when a new frame starts (via frame detection strategies)
2. Truncating the buffer at frame boundaries, keeping only the latest frame
3. Providing snapshot reads that force a clean redraw

Enable with `--tui` on session creation:
```bash
shelli create myapp --cmd htop --tui
```

## Frame Detection

The `FrameDetector` (`internal/ansi/clear.go`) processes PTY output chunks and identifies frame boundaries using five strategies, checked in priority order.

### Strategy 1: Screen Clear (`screen_clear`)

**Trigger**: `ESC[2J` (clear screen), `ESC[?1049h` (alt buffer), `ESC c` (terminal reset)

**Behavior**: Unconditional truncation. Everything before the sequence is discarded.

**Apps**: vim (alt buffer), less (alt buffer), most TUI apps at startup

### Strategy 2: Sync Mode (`sync_mode`)

**Trigger**: `ESC[?2026h` (synchronized update begin)

**Behavior**: Truncation on frame START. Content between sync begin and sync end forms one frame. Suppressed during snapshot mode to allow partial redraws to accumulate.

**Apps**: lazygit, Claude Code, modern terminals

### Strategy 3: Cursor Home (`cursor_home`)

**Trigger**: `ESC[1;1H` or `ESC[H` (cursor to position 1,1)

**Behavior**: Only fires when preceded by a heuristic marker within 20 bytes:
- `ESC[0m` or `ESC[m` (attribute reset)
- `ESC[?25l` (hide cursor)

Additionally uses look-ahead to distinguish real frame boundaries from cursor repositioning (e.g., vim/micro editing cursor returning to row 1):
- If printable content follows within 50 bytes: truncate (real frame)
- If only cursor control sequences follow (`ESC[?25h`, `ESC[?12l`, etc.): skip
- If at end of chunk (ambiguous): defer decision to next chunk

A within-chunk cooldown of 4096 bytes prevents double-firing when apps send multiple cursor_home sequences within a single render pass.

**Apps**: k9s, htop, nnn

### Strategy 4: Cursor Jump to Top (`CursorJumpTop`)

**Trigger**: `ESC[row;colH` where `row <= 2` and `maxRowSeen >= 10`

**Behavior**: Detects when cursor jumps from a high row back to the top of the screen, indicating a full screen redraw. Uses look-ahead to distinguish real frame boundaries from cursor repositioning:
- If printable content follows (possibly after color/mode sequences): truncate
- If only cursor control sequences follow (`ESC[?25h`, `ESC[?12l`, etc.): skip
- If at end of chunk (ambiguous): defer decision to next chunk

**Apps**: htop, glances, apps that draw rows sequentially

### Strategy 5: Size Cap (`MaxSize`)

**Trigger**: Buffer exceeds `MaxSize` (default 100KB)

**Behavior**: Only fires if a frame boundary was detected recently (within `MaxSize * 2` bytes). This prevents breaking hybrid apps (like btm) that send one frame at startup then switch to incremental updates.

**Apps**: Safety net for any app with recent frame detection

## Snapshot Mechanism

Snapshot (`--snapshot` on read) provides a clean, current frame by forcing a full redraw.

### Flow

1. **Cold start wait**: If storage is empty, wait up to 2s for initial content (handles slow-starting apps)
2. **Clear storage** and reset frame detector
3. **Enable snapshot mode** (suppresses ALL truncation strategies)
4. **Resize cycle**: Set terminal to (cols+1, rows+1), send SIGWINCH, pause 200ms, restore original size, send SIGWINCH
5. **Settle loop**: Poll storage every 25ms until content stops changing for `settle_ms` (default 300ms)
6. **Retry**: If output is still empty, send another SIGWINCH with 2x settle time
7. **Disable snapshot mode** and return output

### Why resize?

TUI apps listen for SIGWINCH (window size change) and perform a full redraw. By temporarily changing the size and changing it back, we trigger two redraws. The frame detector captures the clean output from the final redraw.

## Virtual Screen Buffer

The virtual screen buffer (`internal/ansi/strip.go`) converts cursor-positioned terminal output into readable linear text. Used when `--strip-ansi` is applied to read output.

### How it works

1. Pre-scan all cursor positioning sequences to determine grid dimensions
2. Allocate a rune-based grid (supports multi-byte UTF-8: box-drawing, emoji, CJK)
3. Process the string, executing cursor movements and writing characters to grid cells
4. Output: join grid rows, right-trim trailing spaces, remove trailing empty rows

### Supported sequences

| Sequence | Name | Behavior |
|----------|------|----------|
| `ESC[row;colH` / `ESC[row;colF` | Cursor Position | Move to absolute row, col |
| `ESC[nH` / `ESC[H` | Cursor Row / Home | Move to row n (or 1,1) |
| `ESC[nG` | Cursor Column Absolute | Move to column n, keep row |
| `ESC[nd` | Cursor Row Absolute | Move to row n, keep column |
| `ESC[nA` | Cursor Up | Move up n rows |
| `ESC[nB` | Cursor Down | Move down n rows |
| `ESC[nC` | Cursor Right | Move right n columns |
| `ESC[nD` | Cursor Left | Move left n columns |
| `ESC[K` / `ESC[0K` | Erase to End | Clear from cursor to end of line |
| `ESC[1K` | Erase to Start | Clear from start of line to cursor |
| `ESC[2K` | Erase Full Line | Clear entire line |
| `ESC(0` | DEC Graphics On | Activate DEC Special Graphics charset |
| `ESC(B` | DEC Graphics Off | Deactivate, return to ASCII |

### DEC Special Graphics

When `ESC(0` is active, ASCII characters are mapped to box-drawing glyphs:

| Input | Output | Description |
|-------|--------|-------------|
| `q` | `─` | Horizontal line |
| `x` | `│` | Vertical line |
| `l` | `┌` | Top-left corner |
| `k` | `┐` | Top-right corner |
| `m` | `└` | Bottom-left corner |
| `j` | `┘` | Bottom-right corner |
| `n` | `┼` | Cross |
| `t` | `├` | Left tee |
| `u` | `┤` | Right tee |
| `v` | `┴` | Bottom tee |
| `w` | `┬` | Top tee |

## Terminal Responder

The `TerminalResponder` (`internal/ansi/responder.go`) intercepts terminal capability queries in PTY output and writes responses back to the PTY input. This unblocks apps that wait for query responses before rendering.

### Intercepted queries

| Query | Sequence | Response |
|-------|----------|----------|
| DA1 (Primary Device Attributes) | `ESC[c` / `ESC[0c` | `ESC[?62;22c` (VT220 with ANSI color) |
| DA2 (Secondary Device Attributes) | `ESC[>c` / `ESC[>0c` | `ESC[>1;1;0c` (VT220, version 1) |
| DSR (Cursor Position Report) | `ESC[6n` | `ESC[rows;colsR` (current terminal size) |
| Kitty Keyboard Query | `ESC[?u` | `ESC[?0u` (not supported) |
| DECRPM (Mode Report) | `ESC[?{n}$p` | `ESC[?{n};0$y` (not recognized) |

## App Compatibility

| App | Frame Detection | Snapshot | Strip-ANSI | Score | Notes |
|-----|----------------|----------|------------|-------|-------|
| btop | cursor_home | Clean | Good | 9/9 | HVP cursor (CSI f) support |
| htop | screen_clear | Clean | Good | 9/9 | Reference-quality TUI support |
| glances | CursorJumpTop | Clean | Good | 9/9 | Needs --settle 3000 for consistency |
| k9s | cursor_home | Clean | Partial | 9/9 | Left columns truncated in strip-ansi |
| ranger | CursorJumpTop | Clean | Good | 9/9* | Fixed by snapshot truncation suppression |
| nnn | cursor_home | Clean | Good | 9/9 | |
| yazi | screen_clear | Clean | Partial | 9/9 | Only right pane in strip-ansi |
| vifm | cursor_home | Clean | Good | 9/9 | |
| lazygit | sync_mode | Clean | Good | 9/9 | |
| tig | screen_clear | Clean | Good | 9/9 | |
| vim | screen_clear + cursor_home | Clean | Good | 9/9 | Fixed by cursor_home look-ahead + snapshot suppression |
| less | screen_clear | Clean | Good | 9/9* | Fixed by newline grid sizing |
| micro | CursorJumpTop + cursor_home | Clean | Good | 9/9 | Fixed by cursor_home look-ahead + snapshot suppression |
| weechat | cursor_home | Clean | Good | 9/9 | Timing-dependent at large sizes |
| irssi | screen_clear | Clean | Good | 9/9 | |
| newsboat | screen_clear | Clean | Minor gaps | 8/9 | Help screen capture fails |
| mc | cursor_home | Clean | Good | 9/9 | Heavy box drawing renders well |
| bat | screen_clear | Clean | Good | 9/9* | Fixed by newline grid sizing |
| aerc | N/A | N/A | N/A | SKIP | Needs email config |

*= fixed by snapshot truncation suppression or newline grid sizing

## Known Limitations

### Scroll Regions (DECSTBM)

`ESC[top;bottomr` sets a scroll region. The virtual screen buffer does not track scroll regions, so apps that use scrolling within a region (e.g., some panels in mc) may have degraded strip-ansi output.

### Direct /dev/tty Access

Some apps open `/dev/tty` directly instead of using stdin/stdout. These bypass the PTY entirely, making shelli unable to capture their output or respond to terminal queries. Most apps work fine, but this is a known edge case.

### Wide Characters

CJK characters and some emoji occupy two terminal cells but one grid position. The virtual screen buffer currently treats each character as one cell, which may cause alignment issues with double-width characters.

### Partial Strip-ANSI for Complex Layouts

Some apps with complex multi-pane layouts produce partial strip-ansi output:
- **k9s**: Left-side columns may be truncated in strip-ansi snapshots
- **yazi**: Only the right pane renders in strip-ansi (raw snapshots are complete)

## Constants Reference

| Constant | Value | Location | Purpose |
|----------|-------|----------|---------|
| `maxSequenceLen` | 12 | `clear.go` | Max escape sequence length for cross-chunk buffering |
| `cursorHomeLookback` | 20 bytes | `clear.go` | Lookback window for cursor_home heuristic markers |
| `cursorJumpTopThreshold` | 10 rows | `clear.go` | Minimum maxRowSeen before jump-to-top triggers |
| `jumpLookAheadLen` | 50 bytes | `clear.go` | Bytes to scan forward after cursor jump |
| `cursorHomeCooldownBytes` | 4096 bytes | `clear.go` | Within-chunk cooldown after cursor_home fires |
| `MaxSize` (default) | 100KB | `clear.go` | Size cap fallback threshold |
| `maxGridCols` | 500 | `strip.go` | Maximum virtual screen buffer columns |
| `maxGridRows` | 500 | `strip.go` | Maximum virtual screen buffer rows |
| `DefaultSnapshotSettleMs` | 300ms | `constants.go` | Default settle time for snapshot |
| `SnapshotPollInterval` | 25ms | `constants.go` | Polling interval during snapshot settle |
| `SnapshotResizePause` | 200ms | `constants.go` | Pause between resize steps |
