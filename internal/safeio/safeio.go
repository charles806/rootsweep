// Package safeio performs file moves that can never overwrite an existing
// destination.
package safeio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrDestinationExists is returned when the destination already exists.
var ErrDestinationExists = errors.New("destination exists")

// ErrNotRegularFile is returned when the source is missing or is not a
// regular file (symlinks, directories, devices, etc.).
var ErrNotRegularFile = errors.New("source is not a regular file")

// ErrOutsideRoot is returned when a path escapes its allowed root.
var ErrOutsideRoot = errors.New("path is outside the selected root")

// Within reports whether path is inside root. Both are cleaned first.
func Within(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// MoveNoOverwrite moves the regular file at src to dst without ever
// overwriting an existing destination.
//
// Strategy: hard-link src to the fresh destination path — os.Link fails with
// EEXIST instead of clobbering — then unlink the source. Both paths must be
// on the same filesystem; RootSweep only moves files inside the selected
// root, so this always holds. The destination is rechecked immediately
// before every move. If the unlink fails, the new link is removed again so
// the move is all-or-nothing and no data is lost.
func MoveNoOverwrite(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrNotRegularFile, src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	// The recheck that matters: os.Link never overwrites, so an existing dst
	// fails here instead of being clobbered by a rename.
	if err := os.Link(src, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, dst)
		}
		return fmt.Errorf("link: %w", err)
	}
	if err := os.Remove(src); err != nil {
		_ = os.Remove(dst) // roll back: the move must be all-or-nothing
		return fmt.Errorf("remove source: %w", err)
	}
	return nil
}
