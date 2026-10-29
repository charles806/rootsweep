# RootSweep

**RootSweep** is an interactive terminal file organizer written in Go. It scans a directory, detects exact duplicate files using SHA-256, proposes moves for loose files at the root into category folders, and executes only the moves you explicitly confirm. It never deletes or overwrites files.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) for a responsive, full-screen terminal interface.

---

## Features

- **Interactive terminal dashboard** — Full-screen TUI with live progress during scanning, hashing, and execution.
- **Duplicate detection** — Finds exact byte-for-byte duplicates using SHA-256, grouped by size first to minimize hashing work.
- **File-type categorization** — Proposes moves into eight categories: Documents, Images, Videos, Audio, Archives, Installers, Code, and Other.
- **Review before execution** — Select individual moves or all eligible moves, then review a summary before confirming.
- **Collision protection** — Destinations that already exist are flagged and skipped; they are never overwritten.
- **Duplicate awareness** — Files belonging to duplicate groups are excluded from move proposals; you decide what to do with them separately.
- **No-overwrite guarantee** — Moves use a hard-link-then-unlink strategy that fails atomically if the destination appears between planning and execution.
- **Safe by design** — Read-only until you press `y` on the confirmation screen. No background daemons, no automatic cleanup.

---

## How It Works

1. **Scan** — Recursively walks the target folder, collecting eligible regular files. Skips hidden files, `.git`, `node_modules`, `__MACOSX`, `.DS_Store`, AppleDouble `._*` files, symlinks, and special files.
2. **Hash** — Groups files by size; only size-colliding files are hashed (SHA-256, streamed in 32 KiB chunks). A bounded worker pool (1–8 workers, based on CPU count) runs in parallel with a live progress bar.
3. **Plan** — For each eligible file sitting directly in the root (depth 1), proposes a move into the appropriate category folder. Files in subdirectories, duplicate-group members, and files with existing destinations are excluded or marked as conflicts.
4. **Review** — Navigate the list with ↑/↓, press Space to select/deselect, `a` to select all eligible moves, then Enter to review the summary.
5. **Confirm** — Press `y` to execute, `n` to cancel and return to the list.
6. **Execute** — Approved moves run sequentially with progress reporting. Each move is rechecked for conflicts at execution time. Results show moved, skipped (conflict), or failed.

---

## Categories

| Category | Extensions (examples) |
|----------|----------------------|
| **Documents** | `.pdf`, `.doc`, `.docx`, `.txt`, `.md`, `.rtf`, `.odt`, `.ods`, `.xls`, `.xlsx`, `.csv`, `.ppt`, `.pptx`, `.pages`, `.numbers`, `.key`, `.tex`, `.epub`, `.mobi`, `.msg`, `.eml` |
| **Images** | `.jpg`, `.jpeg`, `.png`, `.gif`, `.bmp`, `.svg`, `.webp`, `.tiff`, `.tif`, `.ico`, `.heic`, `.heif`, `.raw`, `.cr2`, `.nef`, `.arw`, `.psd`, `.avif` |
| **Videos** | `.mp4`, `.mov`, `.avi`, `.mkv`, `.wmv`, `.flv`, `.webm`, `.m4v`, `.mpg`, `.mpeg`, `.3gp`, `.mts`, `.m2ts`, `.vob`, `.ogv` |
| **Audio** | `.mp3`, `.wav`, `.flac`, `.aac`, `.ogg`, `.oga`, `.m4a`, `.wma`, `.aiff`, `.aif`, `.opus`, `.mid`, `.midi`, `.mka` |
| **Archives** | `.zip`, `.rar`, `.7z`, `.tar`, `.gz`, `.tgz`, `.bz2`, `.xz`, `.zst`, `.iso`, `.cab`, `.lz`, `.lzma`, `.z`, `.arj`, `.ace` |
| **Installers** | `.dmg`, `.pkg`, `.mpkg` |
| **Code** | `.go`, `.py`, `.js`, `.mjs`, `.ts`, `.tsx`, `.jsx`, `.java`, `.c`, `.h`, `.cpp`, `.cc`, `.hpp`, `.cs`, `.rb`, `.php`, `.rs`, `.swift`, `.kt`, `.kts`, `.scala`, `.sh`, `.bash`, `.zsh`, `.fish`, `.pl`, `.pm`, `.lua`, `.r`, `.jl`, `.m`, `.sql`, `.html`, `.htm`, `.css`, `.scss`, `.sass`, `.less`, `.json`, `.jsonl`, `.yaml`, `.yml`, `.toml`, `.xml`, `.ini`, `.cfg`, `.conf`, `.env`, `.gradle`, `.vue`, `.svelte`, `.ex`, `.exs`, `.erl`, `.hrl`, `.clj`, `.cljs`, `.fs`, `.fsx`, `.dart`, `.ipynb`, `.proto`, `.tf`, `.dockerfile` |
| **Other** | Everything else (no extension or unrecognized) |

Matching is case-insensitive. Unknown extensions fall back to **Other**.

---

## Safety

RootSweep is designed around a few non-negotiable rules:

- **Never deletes files** — Duplicate detection only *reports* duplicate groups; it does not remove them. You decide what to do with the information.
- **Never overwrites** — The move operation uses `os.Link` (hard link) which fails with `EEXIST` if the destination already exists. The destination is rechecked immediately before every move. If a conflict appears between planning and execution, the move is reported as *skipped* and the source file is preserved.
- **Read-only until confirmed** — Scanning, hashing, and planning are all read-only. No filesystem changes happen until you press `y` on the confirmation screen.
- **No symlink following** — Symlinks are skipped entirely during scanning and are never used as move sources.
- **Stays within root** — All moves are validated to remain inside the selected root directory. Paths that would escape are rejected.
- **All-or-nothing moves** — If the unlink step fails after a successful link, the new link is removed so no data is lost and the source remains.

---

## Prerequisites

- **Go 1.26.0 or later** (see `go.mod`)

No external system dependencies.

---

## Installation

### From source

```bash
git clone https://github.com/charles806/rootsweep.git
cd rootsweep
go build -o rootsweep ./cmd/rootsweep
```

The binary will be created at `./rootsweep`. Move it to a directory in your `$PATH` if desired.

### Run without installing

```bash
go run ./cmd/rootsweep <folder>
```

---

## Usage

```bash
rootsweep <folder>
```

- `<folder>` — Path to the directory you want to organize. Can be an absolute path, relative path, or a symlink to a directory (the symlink is resolved).
- Run with `-h` or `--help` to see the usage message.

### Example

```bash
# Organize your Downloads folder
rootsweep ~/Downloads
```

The dashboard opens in the alternate screen buffer. Press `q` at any time to quit safely.

---

## Keyboard Controls

| Phase | Key | Action |
|-------|-----|--------|
| **Ready** (move list) | `↑` / `↓` | Navigate the proposed move list |
| | `Space` | Select/deselect the highlighted move (skips conflicts) |
| | `a` | Select all eligible (non-conflicting) moves |
| | `Enter` | Open the confirmation screen with selected moves |
| | `q` / `Esc` / `Ctrl+C` | Quit |
| **Confirm** | `y` | Confirm and execute the selected moves |
| | `n` | Cancel and return to the move list |
| | `q` / `Esc` / `Ctrl+C` | Quit (no moves executed) |
| **Execute / Done** | `q` / `Esc` / `Ctrl+C` | Quit |

---

## Development

### Run tests

```bash
go test -race -count=1 ./...
```

Tests use temporary directories and generated fixtures only. The `-race` flag enables the race detector.

### Format code

```bash
gofmt -w .
```

### Project structure

```
cmd/rootsweep/main.go       Application startup and CLI validation
internal/scanner/           Recursive folder scanning with exclusions
internal/dedup/             Size grouping + SHA-256 duplicate detection
internal/organizer/         File-type categorization and move planning
internal/safeio/            No-overwrite move primitives (hard-link strategy)
internal/executor/          Approved-move execution with progress
internal/ui/                Bubble Tea dashboard (alt-screen TUI)
```

---

## Limitations

- **Same-filesystem moves only** — Moves use hard links, so source and destination must reside on the same filesystem. Since RootSweep only moves files within the selected root, this is always true in practice.
- **Conflict detection is plan-time** — Destinations that appear between planning and execution are reported as skipped, never overwritten.
- **Depth-1 only** — Only files directly inside the chosen root are proposed for moves. Nested files are scanned for duplicates but never moved.
- **No recursive organization** — RootSweep does not descend into subdirectories to organize their contents.
- **No undo** — Once a move is confirmed and executed, there is no built-in undo. The original file is gone (moved to the category folder).

---

## Contributing

Issues and pull requests are welcome. Before opening a PR:

1. Run `go test -race -count=1 ./...` and ensure all tests pass.
2. Run `gofmt -w .` to format code.
3. Keep changes focused and minimal. The project intentionally avoids scope creep.

---

## License

This project does not currently specify a license. All rights reserved.