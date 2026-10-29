package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildTree(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "sub", "b.txt"), "b")
	writeFile(t, filepath.Join(root, "sub", "deep", "c.txt"), "c")
	// Excluded entries.
	writeFile(t, filepath.Join(root, ".hidden"), "h")
	writeFile(t, filepath.Join(root, ".DS_Store"), "ds")
	writeFile(t, filepath.Join(root, "._apple"), "ad")
	writeFile(t, filepath.Join(root, ".git", "config"), "g")
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "n")
	writeFile(t, filepath.Join(root, "__MACOSX", "_.junk"), "m")
	writeFile(t, filepath.Join(root, "sub", ".gitignored"), "sg")
	// Symlink must be skipped, not followed.
	if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	return root
}

func findFile(t *testing.T, files []File, suffix string) File {
	t.Helper()
	for _, f := range files {
		if filepath.Base(f.Path) == suffix {
			return f
		}
	}
	t.Fatalf("file %q not found in %+v", suffix, files)
	return File{}
}

func TestScanCollectsEligibleFiles(t *testing.T) {
	root := buildTree(t)
	result, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("expected 3 eligible files, got %d: %+v", len(result.Files), result.Files)
	}
	a := findFile(t, result.Files, "a.txt")
	if a.Depth != 1 {
		t.Errorf("a.txt depth = %d, want 1", a.Depth)
	}
	b := findFile(t, result.Files, "b.txt")
	if b.Depth != 2 {
		t.Errorf("b.txt depth = %d, want 2", b.Depth)
	}
	c := findFile(t, result.Files, "c.txt")
	if c.Depth != 3 {
		t.Errorf("c.txt depth = %d, want 3", c.Depth)
	}
	if result.SymlinksSkipped != 1 {
		t.Errorf("symlinks skipped = %d, want 1", result.SymlinksSkipped)
	}
	if result.DirectCount() != 1 {
		t.Errorf("direct count = %d, want 1", result.DirectCount())
	}
}

func TestScanSkipsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "regular.txt"), "r")
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dir", "inner.txt"), []byte("i"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(result.Files), result.Files)
	}
}

func TestScanReportsUnreadableEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ok.txt"), "ok")
	if err := os.Mkdir(filepath.Join(root, "locked"), 0o000); err != nil {
		t.Skipf("running as privileged user: %v", err)
	}
	defer os.Chmod(filepath.Join(root, "locked"), 0o755)
	result, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if len(result.ScanErrors) == 0 {
		t.Error("expected an error for the unreadable directory")
	}
}

func TestScanEmitCallback(t *testing.T) {
	root := buildTree(t)
	var emitted int
	if _, err := Scan(root, func(File) { emitted++ }); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if emitted != 3 {
		t.Errorf("emit called %d times, want 3", emitted)
	}
}

func TestValidateRoot(t *testing.T) {
	dir := t.TempDir()
	got, err := ValidateRoot(dir)
	if err != nil {
		t.Fatalf("ValidateRoot(dir): %v", err)
	}
	if got != dir && filepath.Clean(got) != filepath.Clean(dir) {
		t.Errorf("ValidateRoot = %q, want %q", got, dir)
	}

	if _, err := ValidateRoot(filepath.Join(dir, "missing")); err == nil {
		t.Error("expected error for nonexistent path")
	}

	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRoot(file); err == nil {
		t.Error("expected error for a file (not a directory)")
	}

	// A symlink to a directory resolves to the real directory.
	link := filepath.Join(dir, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	got, err = ValidateRoot(link)
	if err != nil {
		t.Fatalf("ValidateRoot(symlink): %v", err)
	}
	if got == link {
		t.Errorf("expected symlink to be resolved, got %q", got)
	}
}
