// Package scanner walks a folder tree and collects eligible regular files.
package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// File describes one eligible regular file found during a scan.
type File struct {
	Path  string // absolute path
	Size  int64  // size in bytes
	Depth int    // 1 = directly inside the root, >1 = nested in a subdirectory
}

// ScanResult aggregates everything a scan discovered.
type ScanResult struct {
	Root            string
	Files           []File
	ScanErrors      []error // unreadable entries that were skipped
	SymlinksSkipped int
	SpecialSkipped  int // non-regular, non-symlink entries (devices, sockets, fifos)
}

// EligibleCount is the number of eligible regular files found.
func (r *ScanResult) EligibleCount() int { return len(r.Files) }

// DirectCount is the number of eligible files directly inside the root.
func (r *ScanResult) DirectCount() int {
	n := 0
	for _, f := range r.Files {
		if f.Depth == 1 {
			n++
		}
	}
	return n
}

// neverDescend lists directory names that are always skipped entirely.
var neverDescend = map[string]bool{
	".git":         true,
	"node_modules": true,
	"__MACOSX":     true,
}

// IsHidden reports whether a name is hidden (starts with a dot).
func IsHidden(name string) bool { return strings.HasPrefix(name, ".") }

// IsAppleDouble reports whether a name is an AppleDouble "._*" file.
func IsAppleDouble(name string) bool { return strings.HasPrefix(name, "._") }

// IsSkippedDirName reports whether a directory should never be entered.
func IsSkippedDirName(name string) bool {
	return IsHidden(name) || neverDescend[name]
}

// IsSkippedFileName reports whether a regular file should be ignored.
// Hidden files (which already covers .DS_Store) and AppleDouble "._*"
// files are skipped.
func IsSkippedFileName(name string) bool {
	return IsHidden(name) || IsAppleDouble(name)
}

// ValidateRoot checks that path exists and is a directory. A symlink pointing
// at a directory is resolved so users can pass a link to their folder; the
// returned path is absolute.
func ValidateRoot(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		info, err = os.Lstat(resolved)
		if err != nil {
			return "", err
		}
		path = resolved
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// Scan walks root recursively and collects every eligible regular file.
// emit, when non-nil, is called for each file as it is found.
//
// Skipped entirely: hidden files and directories, .git, node_modules,
// __MACOSX, .DS_Store, and AppleDouble "._*" files. Symbolic links and
// other special files are never followed or processed.
func Scan(root string, emit func(File)) (*ScanResult, error) {
	result := &ScanResult{Root: root}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			result.ScanErrors = append(result.ScanErrors, fmt.Errorf("%s: %w", path, err))
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && IsSkippedDirName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			result.SymlinksSkipped++
			return nil
		case !d.Type().IsRegular():
			result.SpecialSkipped++
			return nil
		}
		if IsSkippedFileName(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			result.ScanErrors = append(result.ScanErrors, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			result.ScanErrors = append(result.ScanErrors, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		f := File{
			Path:  path,
			Size:  info.Size(),
			Depth: 1 + strings.Count(rel, string(os.PathSeparator)),
		}
		result.Files = append(result.Files, f)
		if emit != nil {
			emit(f)
		}
		return nil
	})
	return result, err
}
