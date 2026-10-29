package executor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"rootsweep/internal/organizer"
	"rootsweep/internal/safeio"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteMovesApprovedFiles(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.txt")
	write(t, src, "aaa")
	move := organizer.Move{Source: src, Dest: filepath.Join(root, "Documents", "a.txt"), Category: organizer.Documents, Size: 3}

	results := Execute(root, []organizer.Move{move}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != Moved || r.Err != nil {
		t.Fatalf("status = %s, err = %v", r.Status, r.Err)
	}
	if _, err := os.ReadFile(filepath.Join(root, "Documents", "a.txt")); err != nil {
		t.Errorf("moved file missing at destination: %v", err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("source should be gone, err = %v", err)
	}
}

func TestExecuteReportsConflictAsBlocked(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.txt")
	dest := filepath.Join(root, "Documents", "a.txt")
	write(t, src, "new")
	write(t, dest, "precious")

	move := organizer.Move{Source: src, Dest: dest, Category: organizer.Documents, Size: 3}
	results := Execute(root, []organizer.Move{move}, nil)
	r := results[0]
	if r.Status != Blocked {
		t.Fatalf("status = %s, want blocked", r.Status)
	}
	if !errors.Is(r.Err, safeio.ErrDestinationExists) {
		t.Errorf("err = %v, want ErrDestinationExists", r.Err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "precious" {
		t.Errorf("destination overwritten: %q", got)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Errorf("source lost on blocked move: %v", err)
	}
}

func TestExecuteContinuesAfterFailure(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.txt")
	write(t, good, "ggg")
	vanished := filepath.Join(root, "vanished.txt") // never created
	moves := []organizer.Move{
		{Source: vanished, Dest: filepath.Join(root, "Other", "vanished.txt"), Category: organizer.Other, Size: 0},
		{Source: good, Dest: filepath.Join(root, "Documents", "good.txt"), Category: organizer.Documents, Size: 3},
	}
	results := Execute(root, moves, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != Failed || results[0].Err == nil {
		t.Errorf("first move: status=%s err=%v, want failed with error", results[0].Status, results[0].Err)
	}
	if results[1].Status != Moved {
		t.Errorf("second move should still succeed: status=%s err=%v", results[1].Status, results[1].Err)
	}
}

func TestExecuteRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	src := filepath.Join(root, "a.txt")
	write(t, src, "aaa")
	move := organizer.Move{Source: src, Dest: filepath.Join(outside, "a.txt"), Size: 3}
	results := Execute(root, []organizer.Move{move}, nil)
	if results[0].Status != Failed {
		t.Fatalf("status = %s, want failed", results[0].Status)
	}
	if !errors.Is(results[0].Err, safeio.ErrOutsideRoot) {
		t.Errorf("err = %v, want ErrOutsideRoot", results[0].Err)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Errorf("source must be preserved: %v", err)
	}
	if _, err := os.Lstat(move.Dest); !os.IsNotExist(err) {
		t.Errorf("nothing may appear outside the root: %v", err)
	}
}

func TestExecuteProgress(t *testing.T) {
	root := t.TempDir()
	var moves []organizer.Move
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		src := filepath.Join(root, name)
		write(t, src, "x")
		moves = append(moves, organizer.Move{Source: src, Dest: filepath.Join(root, "Other", name), Category: organizer.Other})
	}
	var calls int
	Execute(root, moves, func(done, total int) {
		if done != calls+1 || total != 3 {
			t.Errorf("progress = (%d, %d), want (%d, 3)", done, total, calls+1)
		}
		calls++
	})
	if calls != 3 {
		t.Errorf("progress calls = %d, want 3", calls)
	}
}
