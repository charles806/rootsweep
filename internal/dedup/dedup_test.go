package dedup

import (
	"os"
	"path/filepath"
	"testing"

	"rootsweep/internal/scanner"
)

func newFiles(t *testing.T, specs map[string]string) []scanner.File {
	t.Helper()
	var files []scanner.File
	for name, content := range specs {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, scanner.File{Path: path, Size: int64(len(content)), Depth: 1})
	}
	return files
}

func TestIdenticalContentDifferentNamesDetected(t *testing.T) {
	content := "identical payload"
	files := newFiles(t, map[string]string{
		"a.txt": content,
		"b.log": content,
		"c.md":  content,
	})
	report := FindDuplicates(files, 2, nil)
	if len(report.Groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(report.Groups))
	}
	g := report.Groups[0]
	if len(g.Files) != 3 {
		t.Fatalf("group has %d files, want 3: %v", len(g.Files), g.Files)
	}
	if g.ExtraCopies() != 2 {
		t.Errorf("extra copies = %d, want 2", g.ExtraCopies())
	}
	if g.ExtraBytes() != 2*int64(len(content)) {
		t.Errorf("extra bytes = %d, want %d", g.ExtraBytes(), 2*len(content))
	}
	if report.ExtraCopyCount() != 2 {
		t.Errorf("report extra copies = %d, want 2", report.ExtraCopyCount())
	}
	dup := report.DupPaths()
	for _, f := range files {
		if !dup[f.Path] {
			t.Errorf("%s missing from DupPaths", f.Path)
		}
	}
}

func TestDifferentContentSameSizeNotDuplicate(t *testing.T) {
	files := newFiles(t, map[string]string{
		"one.bin": "aaaa",
		"two.bin": "bbbb",
	})
	report := FindDuplicates(files, 2, nil)
	if len(report.Groups) != 0 {
		t.Fatalf("expected no duplicate groups, got %d: %+v", len(report.Groups), report.Groups)
	}
	if report.ExtraBytes() != 0 {
		t.Errorf("extra bytes = %d, want 0", report.ExtraBytes())
	}
}

func TestUniqueSizesNeverHashed(t *testing.T) {
	files := newFiles(t, map[string]string{
		"small.txt": "x",
		"big.txt":   "xxxxxxxxxx",
	})
	report := FindDuplicates(files, 2, nil)
	if len(report.Groups) != 0 {
		t.Fatalf("expected no groups, got %d", len(report.Groups))
	}
	if report.HashedFiles != 0 {
		t.Errorf("hashed files = %d, want 0", report.HashedFiles)
	}
}

func TestProgressCallbackReachesTotal(t *testing.T) {
	files := newFiles(t, map[string]string{
		"d1": "dup",
		"d2": "dup",
		"u1": "solo",
	})
	var last Progress
	report := FindDuplicates(files, 2, func(p Progress) {
		if p.Done < 0 || p.Done > p.Total {
			t.Errorf("progress out of range: %+v", p)
		}
		last = p
	})
	if len(report.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(report.Groups))
	}
	if last.Done != 2 || last.Total != 2 {
		t.Errorf("final progress = %+v, want {2 2}", last)
	}
}

func TestHashErrorsRecorded(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(good, []byte("twin"), 0o644); err != nil {
		t.Fatal(err)
	}
	vanished := filepath.Join(dir, "vanished.txt")
	// The file has a size collision with good.txt but disappears before hashing.
	if err := os.WriteFile(vanished, []byte("twin"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []scanner.File{{Path: good, Size: 4}, {Path: vanished, Size: 4}}
	if err := os.Remove(vanished); err != nil {
		t.Fatal(err)
	}
	report := FindDuplicates(files, 1, nil)
	if len(report.HashErrors) != 1 {
		t.Fatalf("expected 1 hash error, got %d: %v", len(report.HashErrors), report.HashErrors)
	}
	if len(report.Groups) != 0 {
		t.Fatalf("expected 0 groups when a twin is unreadable, got %d", len(report.Groups))
	}
}

func TestEmptyInput(t *testing.T) {
	report := FindDuplicates(nil, 4, nil)
	if len(report.Groups) != 0 || report.HashedFiles != 0 {
		t.Errorf("empty input should yield empty report, got %+v", report)
	}
}
