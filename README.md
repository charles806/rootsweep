# RootSweep — Go File Organizer

An interactive terminal file organizer. RootSweep scans a folder for duplicate
files and proposes file-type-based moves. It is strictly read-only until you
explicitly confirm moves, and it never deletes or overwrites files.

## Launch

```sh
go run ./cmd/rootsweep <folder>
# or
go build -o rootsweep ./cmd/rootsweep
./rootsweep <folder>
```

## What it does

1. **Scans** your folder recursively, skipping hidden files, `.git`,
   `node_modules`, `__MACOSX`, `.DS_Store`, and AppleDouble `._*` files.
   Symlinks and special files are never followed or processed.
2. **Detects exact duplicates** with SHA-256, grouping by size first and hashing
   only files that could collide. Uses a bounded worker pool with live progress.
3. **Proposes moves** for files directly inside the selected root into
   `Documents`, `Images`, `Videos`, `Audio`, `Archives`, `Installers`, `Code`, or `Other`.
   Duplicate-group members and files with existing destinations are skipped.
4. **Executes approved moves** only after a separate confirmation step, using a
   no-overwrite hard-link strategy that rechecks conflicts before every move.

## Keyboard controls

| Key   | Action                                  |
|-------|-----------------------------------------|
| ↑/↓   | Navigate the move list                  |
| Space | Select/deselect a proposed move         |
| A     | Select all eligible moves               |
| Enter | Review the selected operations          |
| Y     | Confirm the displayed summary           |
| N     | Cancel the operation                    |
| Q     | Quit safely                             |

## Testing

```sh
go test ./...
```

Tests use temporary directories and generated fixtures only.

## Project layout

```
cmd/rootsweep/main.go      application startup and CLI validation
internal/scanner/          recursive folder scanning with exclusions
internal/dedup/            size grouping + SHA-256 duplicate detection
internal/organizer/        file-type categorization and move planning
internal/safeio/           no-overwrite move primitives
internal/executor/         approved-move execution with progress
internal/ui/               Bubble Tea dashboard (alt-screen TUI)
```

## Known limitations

- Moves use hard links, so source and destination must share a filesystem.
  RootSweep only moves files within the selected root, so this always holds.
- Conflict detection is plan-time; destinations that appear between planning
  and execution are reported as skipped, never overwritten.
