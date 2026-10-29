package safeio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveNoOverwriteSucceeds(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "Docs", "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MoveNoOverwrite(src, dst); err != nil {
		t.Fatalf("MoveNoOverwrite: %v", err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("source should be gone, lstat err = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "payload" {
		t.Errorf("destination content = %q, err = %v", got, err)
	}
}

func TestMoveNoOverwriteNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := MoveNoOverwrite(src, dst)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("error = %v, want ErrDestinationExists", err)
	}
	// Both files survive intact.
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "original" {
		t.Errorf("destination overwritten: %q, err = %v", got, err)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Errorf("source lost: %v", err)
	}
}

func TestMoveNoOverwriteRejectsNonRegularSource(t *testing.T) {
	root := t.TempDir()
	// Directory as source.
	if err := MoveNoOverwrite(root, filepath.Join(root, "x")); !errors.Is(err, ErrNotRegularFile) {
		t.Errorf("directory source: err = %v, want ErrNotRegularFile", err)
	}
	// Symlink as source must not be followed.
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := MoveNoOverwrite(link, filepath.Join(root, "y")); !errors.Is(err, ErrNotRegularFile) {
		t.Errorf("symlink source: err = %v, want ErrNotRegularFile", err)
	}
}

func TestMoveNoOverwriteMissingSource(t *testing.T) {
	root := t.TempDir()
	err := MoveNoOverwrite(filepath.Join(root, "gone.txt"), filepath.Join(root, "x"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if errors.Is(err, ErrDestinationExists) || errors.Is(err, ErrNotRegularFile) {
		t.Errorf("wrong error type: %v", err)
	}
}

func TestWithin(t *testing.T) {
	root := "/data/root"
	cases := []struct {
		path string
		want bool
	}{
		{"/data/root", true},
		{"/data/root/file.txt", true},
		{"/data/root/sub/file.txt", true},
		{"/data", false},
		{"/etc/passwd", false},
		{"/data/root/../etc/passwd", false},
	}
	for _, tc := range cases {
		if got := Within(root, tc.path); got != tc.want {
			t.Errorf("Within(%q, %q) = %v, want %v", root, tc.path, got, tc.want)
		}
	}
}

func TestMissingSourceFailsWithoutCreatingDestination(t *testing.T) {
	root := t.TempDir()
	err := MoveNoOverwrite(filepath.Join(root, "gone.txt"), filepath.Join(root, "dst.txt"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if _, err := os.Lstat(filepath.Join(root, "dst.txt")); !os.IsNotExist(err) {
		t.Errorf("no destination should exist, err = %v", err)
	}
}
