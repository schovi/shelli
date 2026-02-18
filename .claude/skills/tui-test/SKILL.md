# TUI Testing Skill

Comprehensive TUI app testing for shelli. Validates create, baseline read, snapshot, snapshot consistency, follow mode, keyboard input, resize, and cleanup across 20 TUI applications using a parallel team of agents.

## Invocation

```
/tui-test [app1 app2 ...]
```

Without arguments: tests all 20 apps. With arguments: tests only the listed apps.

Examples:
```
/tui-test                          # all 20 apps
/tui-test htop lazygit less        # specific apps only
/tui-test --category monitors      # category shorthand (see App Registry)
```

## Prerequisites

- shelli must be built: `make build` (or `go build -o shelli .`)
- Run from the shelli repo root (git tools need a repo)
- Apps installed via `brew install <app>` (orchestrator handles this)

## Orchestration Protocol

The lead agent (you) does the following:

### Phase 1: Setup

1. Build shelli: `make build`
2. Kill any leftover test sessions: `./shelli list` then `./shelli kill test-<name>` for each
3. Determine which apps to test (from args or full registry)
4. Check installation: `which <app>` for each. Install missing ones with `brew install <app>`.
   - Some apps may fail to install or need special setup (k9s needs k8s, aerc needs email config). Mark those as SKIP.
5. Create a team

### Phase 2: Parallel Testing

Spawn one teammate per app (max 5-6 concurrent to avoid system overload). Use `general-purpose` agent type with `bypassPermissions` mode and `model: "sonnet"` (cheaper, sufficient for mechanical test execution).

Each teammate gets:
- The **Teammate Test Protocol** section below (copy it fully into the prompt)
- The app-specific entry from the **App Registry**
- Working directory: the shelli repo root
- Instruction to use `./shelli` (local build)

### Phase 3: Collect Results

As teammates complete, collect their reports. If an app is too broken to test (crashes immediately, doesn't render), note it and optionally spawn a replacement from the same category.

### Phase 4: Final Report

Compile all results into:

1. **Summary table** (all apps, all tests)
2. **Detailed findings** (for each FAIL)
3. **Snapshot samples** (first 15 lines per app)
4. **Overall assessment** (what works well, what's broken, patterns)

### Phase 5: Cleanup

1. Verify all test sessions killed: `./shelli list`
2. Kill any stragglers
3. Shut down teammates, delete team

---

## Teammate Test Protocol

> Copy this entire section into each teammate's prompt.

You are testing shelli's TUI support for a specific app. Run these 9 tests in order. Use `./shelli` (local build) for all commands.

**IMPORTANT**: Always use `--tui` flag on create. Without it, `--snapshot` won't work.

### Test 1: Create Session

```bash
./shelli create test-<app> --cmd "<launch command>" --tui --cols 120 --rows 40
```

Use the launch command from the App Registry entry you were given.

**Pass**: No error. Session appears in `./shelli list`.

### Test 2: Baseline Read

```bash
sleep 3
./shelli read test-<app> --all --strip-ansi
```

Wait 3 seconds for the app to fully initialize (some TUIs take a moment to render).

**Pass**: Non-empty output with recognizable app content (header, status bar, file listing, etc.).

**If empty or garbled**: Try `sleep 5` and read again. Some apps (btop, glances) need extra init time.

### Test 3: Snapshot Read (PRIMARY TEST)

```bash
./shelli read test-<app> --snapshot --strip-ansi
```

**Quality checks**:
1. Output is a SINGLE clean frame (no garbled/overlapping content)
2. Looks like what you'd see on a real terminal running the app
3. Line count roughly matches terminal rows (40), give or take a few
4. No duplicate headers, footers, or status bars
5. Box-drawing characters and UI elements are coherent (not broken mid-line)
6. Content is meaningful (actual data, not just escape code residue)

**If snapshot looks wrong**:
- Try longer settle: `./shelli read test-<app> --snapshot --strip-ansi --settle 1000`
- Try raw read (without --strip-ansi) to check if ANSI stripping is the issue
- Try `./shelli read test-<app> --snapshot --json` for structured metadata
- Check `./shelli info test-<app> --json` for session state

**Record**: Save the first 20 lines of snapshot output in your report.

### Test 4: Snapshot Consistency

Take two snapshots 2 seconds apart:

```bash
SNAP1=$(./shelli read test-<app> --snapshot --strip-ansi)
sleep 2
SNAP2=$(./shelli read test-<app> --snapshot --strip-ansi)
```

**Pass criteria**:
- Both are valid frames
- For live-updating apps (htop, btop, glances): content differs but STRUCTURE is the same (same headers, same layout, same number of panels)
- For static apps (less, ranger on unchanged dir): content is nearly identical
- Neither snapshot is empty or garbled

### Test 5: Follow Mode

```bash
timeout 5 ./shelli read test-<app> --follow --strip-ansi 2>&1; true
```

The `; true` prevents timeout's non-zero exit from being treated as an error.

**Pass**: Output streams without errors. For apps with periodic updates, content flows visibly.

### Test 6: Keyboard Input + Snapshot Verification

Send the key(s) specified in your App Registry entry, then snapshot to verify the app responded:

```bash
./shelli send test-<app> "<key>"
sleep 1
./shelli read test-<app> --snapshot --strip-ansi
```

For multi-step input (e.g., vim commands):
```bash
./shelli send test-<app> "i"           # enter insert mode
sleep 0.5
./shelli send test-<app> "hello world"
sleep 0.5
./shelli send test-<app> "\e"          # escape back to normal
sleep 1
./shelli read test-<app> --snapshot --strip-ansi
```

**Pass**: Snapshot reflects the key action. The change must be visible.

### Test 7: Resize + Snapshot

Resize larger, snapshot, then resize smaller, snapshot:

```bash
./shelli resize test-<app> --cols 160 --rows 50
sleep 2
SNAP_BIG=$(./shelli read test-<app> --snapshot --strip-ansi)
echo "=== LARGE RESIZE ==="
echo "$SNAP_BIG" | head -20

./shelli resize test-<app> --cols 80 --rows 24
sleep 2
SNAP_SMALL=$(./shelli read test-<app> --snapshot --strip-ansi)
echo "=== SMALL RESIZE ==="
echo "$SNAP_SMALL" | head -20
```

**Pass criteria**:
1. Both snapshots are clean frames at their respective dimensions
2. Content reflows correctly (wider lines for large, truncated/wrapped for small)
3. No garbled output from the resize transition

### Test 8: Unicode / Box Drawing

Check the snapshot output for:
- Box-drawing characters (lines, corners, intersections)
- Unicode symbols (arrows, checkmarks, etc.)
- Wide characters (CJK, emoji) if the app displays them

For apps that display files, you can create a test file:
```bash
echo "Box: ┌─┐│└─┘ Emoji: 🚀🔥 Wide: 你好世界" > /tmp/shelli-unicode-test.txt
```

**Pass**: Unicode characters appear correctly in snapshot output (not mojibake, not missing).

Note: This test is informational. PASS if the app renders box drawing or unicode at all. FAIL only if characters are completely mangled.

### Test 9: Cleanup

```bash
./shelli kill test-<app>
./shelli list | grep test-<app>  # should return nothing
```

**Pass**: Session removed, no errors, no orphan processes.

### Debugging Guide

When something doesn't work:

1. `./shelli info test-<app> --json` - check session state, dimensions, PID
2. `./shelli read test-<app> --all` - raw buffer (no --strip-ansi, no --snapshot)
3. `./shelli list --json` - verify process is alive
4. Try `--settle 500`, `--settle 1000`, `--settle 2000` on snapshot
5. Run the app directly to compare: what does the "correct" output look like?
6. If the app is completely non-functional (no output, immediate crash), report SKIP with reason

### Report Format

Return your results in this exact format:

```
## <App Name> Test Results

### Summary

| Test | Result | Notes |
|------|--------|-------|
| 1. Create | PASS/FAIL/SKIP | |
| 2. Baseline | PASS/FAIL/SKIP | |
| 3. Snapshot | PASS/FAIL/SKIP | |
| 4. Consistency | PASS/FAIL/SKIP | |
| 5. Follow | PASS/FAIL/SKIP | |
| 6. Keys | PASS/FAIL/SKIP | |
| 7. Resize | PASS/FAIL/SKIP | |
| 8. Unicode | PASS/FAIL/SKIP | |
| 9. Cleanup | PASS/FAIL/SKIP | |

### Snapshot Sample (first 15 lines)

<paste here>

### Failures (if any)

For each FAIL:
- What happened
- Raw output (first 20 lines)
- Debug steps tried
- Likely root cause
```

---

## App Registry

### Category: System Monitors

Fast redraw, charts, box drawing, periodic updates. Snapshot should always get a clean frame since these apps continuously redraw.

#### btop

- **Install**: `brew install btop`
- **Launch**: `btop`
- **Key test**: Send `m` (cycle memory graph mode)
- **Expected change**: Memory graph visualization changes
- **Notes**: Heavy box drawing, charts, fast updates. Great snapshot stress test.

#### htop

- **Install**: `brew install htop`
- **Launch**: `htop`
- **Key test**: Send `t` (toggle tree view)
- **Expected change**: Process list switches between flat and tree view
- **Notes**: Classic TUI, uses ncurses. Updates every 1-2s.

#### glances

- **Install**: `brew install glances`
- **Launch**: `glances`
- **Key test**: Send `1` (toggle per-CPU stats)
- **Expected change**: CPU section expands/collapses
- **Notes**: Multi-panel dashboard. Python-based TUI. Use `--settle 3000` for consistency test.

#### k9s

- **Install**: `brew install k9s`
- **Launch**: `k9s`
- **Key test**: Send `:` then `namespace\r` (command mode)
- **Expected change**: Namespace view opens
- **Prereq**: Needs a working kubeconfig. If no k8s cluster available, SKIP with note.
- **Notes**: Uses tcell. Complex keybinds. May error without cluster. Left-side columns may be truncated in strip-ansi snapshots.

### Category: File Managers

Lists, previews, navigation keys. Test arrow keys and cursor movement.

#### ranger

- **Install**: `brew install ranger`
- **Launch**: `ranger`
- **Key test**: Send `j` twice then `k` once (down, down, up)
- **Expected change**: File selection cursor moves
- **Notes**: Three-column layout. Preview pane. Python-based. Works after snapshot truncation suppression fix.

#### nnn

- **Install**: `brew install nnn`
- **Launch**: `nnn`
- **Key test**: Send `j` (down)
- **Expected change**: Selection moves to next file
- **Notes**: Minimal TUI. Very fast. Limited box drawing.

#### yazi

- **Install**: `brew install yazi`
- **Launch**: `yazi`
- **Key test**: Send `j` (down)
- **Expected change**: Selection moves to next file
- **Notes**: Rust-based, modern file manager. Rich UI with preview. Only right pane renders in strip-ansi.

#### vifm

- **Install**: `brew install vifm`
- **Launch**: `vifm`
- **Key test**: Send `j` (down)
- **Expected change**: Selection moves in left pane
- **Notes**: Vim-like file manager. Two-pane layout.

### Category: Git / Dev Workflow

Popups, split views, complex keybinds. MUST run from a git repo directory.

#### lazygit

- **Install**: `brew install lazygit`
- **Launch**: `lazygit`
- **Key test**: Send `?` (help overlay)
- **Expected change**: Help popup appears over main view
- **Notes**: Uses tcell. Full redraws. Very clean snapshots expected. Run from shelli repo.

#### tig

- **Install**: `brew install tig`
- **Launch**: `tig`
- **Key test**: Send `j` (move down in log)
- **Expected change**: Commit selection moves down
- **Notes**: ncurses-based git browser. Run from shelli repo.

### Category: Text Editors / Viewers

Modal input, scrolling, search. These stress different aspects of TUI handling.

#### vim

- **Install**: pre-installed on macOS (or `brew install vim`)
- **Launch**: `vim /tmp/shelli-test-vim.txt` (create the file first with some content)
- **Key test**: Send `:set number\r` (turn on line numbers)
- **Expected change**: Line numbers appear in left gutter
- **Setup**: Before create, write a test file: `echo "line1\nline2\nline3\nline4\nline5" > /tmp/shelli-test-vim.txt`
- **Notes**: Alt screen, complex escape sequences. Works after snapshot truncation suppression fix.

#### less

- **Install**: pre-installed on macOS
- **Launch**: `less README.md` (use repo's README)
- **Key test**: Send `G` (jump to end of file)
- **Expected change**: Shows end of file, status bar shows "(END)" or percentage
- **Notes**: Simple pager. Strip-ansi works after newline grid sizing fix.

#### micro

- **Install**: `brew install micro`
- **Launch**: `micro /tmp/shelli-test-micro.txt`
- **Key test**: Type `hello shelli` then snapshot
- **Expected change**: Text appears in editor buffer
- **Setup**: `touch /tmp/shelli-test-micro.txt`
- **Notes**: Modern terminal editor. Uses tcell. Works after snapshot truncation suppression fix.

### Category: Network / Chat

Async events, input + output interleaving. These may need network config.

#### weechat

- **Install**: `brew install weechat`
- **Launch**: `weechat`
- **Key test**: Send `/help\r` (show help)
- **Expected change**: Help text appears in main buffer
- **Prereq**: No IRC server needed for basic UI test. App starts with local buffer.
- **Notes**: Split view with input bar at bottom. Heavy on UI chrome. Timing-dependent inconsistency at larger sizes.

#### irssi

- **Install**: `brew install irssi`
- **Launch**: `irssi`
- **Key test**: Send `/help\r` (show help)
- **Expected change**: Help output in main window
- **Prereq**: No server needed for basic test.
- **Notes**: Classic IRC client. Status bar + input line. Session instability with repeated SIGWINCH.

### Category: Feeds / Disk / Misc

Multi-pane, list-based, longer content. Some need minimal config.

#### ncdu

- **Install**: `brew install ncdu`
- **Launch**: `ncdu /tmp`
- **Key test**: Send `j` (move down in list)
- **Expected change**: Selection moves to next directory/file entry
- **Notes**: Disk usage analyzer. ncurses-based. Clean list UI with size bars. No config needed, works immediately.

#### newsboat

- **Install**: `brew install newsboat`
- **Launch**: `newsboat`
- **Key test**: Send `?` (help)
- **Expected change**: Help screen shown
- **Prereq**: Needs RSS feeds config (~/.config/newsboat/urls). Create a minimal one if missing: `echo "https://hnrss.org/frontpage" > ~/.config/newsboat/urls`
- **Notes**: RSS reader. List-based UI. Help screen capture may fail (8/9 score).

#### mc

- **Install**: `brew install midnight-commander`
- **Launch**: `mc`
- **Key test**: Send `\t` (switch pane)
- **Expected change**: Active pane indicator switches
- **Notes**: Classic two-pane file manager. Heavy box drawing. Norton Commander style.

#### bat

- **Install**: `brew install bat`
- **Launch**: `bat --paging=always README.md`
- **Key test**: Send `G` (jump to end, uses less-style paging)
- **Expected change**: Shows end of file
- **Notes**: Syntax-highlighted pager. Wraps less internally. Strip-ansi works after newline grid sizing fix.

---

## Category Shorthands

For `--category` argument:

| Shorthand | Apps |
|-----------|------|
| monitors | btop, htop, glances, k9s |
| files | ranger, nnn, yazi, vifm |
| git | lazygit, tig |
| editors | vim, less, micro |
| network | weechat, irssi |
| misc | ncdu, newsboat, mc, bat |

---

## Final Report Template

The lead agent compiles all teammate reports into:

### Master Summary Table

| App | Category | Create | Baseline | Snapshot | Consistency | Follow | Keys | Resize | Unicode | Cleanup | Score |
|-----|----------|--------|----------|----------|-------------|--------|------|--------|---------|---------|-------|

Score = number of PASS out of 9.

### Category Summaries

For each category, note:
- Overall quality (all pass, mostly pass, problematic)
- Common issues within the category
- Best and worst performer

### Key Findings

1. **What works well**: Apps/categories with clean snapshot support
2. **What's broken**: Specific failures with root causes
3. **Patterns**: Common failure modes across apps (e.g., "resize breaks X type of apps")
4. **Recommendations**: What to fix in shelli, what to document as limitations

### All Snapshot Samples

Collated first 15 lines of each app's snapshot for visual comparison.
