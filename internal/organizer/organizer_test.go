package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"rootsweep/internal/scanner"
)

func f(path string, depth int) scanner.File {
	return scanner.File{Path: path, Size: 10, Depth: depth}
}

func TestCategoryFor(t *testing.T) {
	cases := map[string]Category{
		"report.pdf":     Documents,
		"notes.TXT":      Documents, // case-insensitive
		"photo.JPG":      Images,
		"clip.mp4":       Videos,
		"song.mp3":       Audio,
		"backup.ZIP":     Archives,
		"backup.7z":      Archives,
		"bundle.rar":     Archives,
		"main.go":        Code,
		"script.PY":      Code,
		"README":         Other,
		"archive.tar.gz": Archives,
		"installer.dmg":  Installers,
		"setup.PKG":      Installers, // case-insensitive
		"bundle.mpkg":    Installers,
		"weird.xyz123":   Other,
	}
	for name, want := range cases {
		if got := CategoryFor(name); got != want {
			t.Errorf("CategoryFor(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestPlanMovesOnlyDirectFiles(t *testing.T) {
	root := t.TempDir()
	direct := filepath.Join(root, "report.pdf")
	nested := filepath.Join(root, "sub", "report.pdf")
	moves := PlanMoves(root, []scanner.File{f(direct, 1), f(nested, 2)}, nil)
	if len(moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(moves))
	}
	if moves[0].Source != direct {
		t.Errorf("source = %q, want %q", moves[0].Source, direct)
	}
	wantDest := filepath.Join(root, "Documents", "report.pdf")
	if moves[0].Dest != wantDest {
		t.Errorf("dest = %q, want %q", moves[0].Dest, wantDest)
	}
	if moves[0].Category != Documents {
		t.Errorf("category = %s, want Documents", moves[0].Category)
	}
}

func TestPlanMovesExcludesDuplicateMembers(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	moves := PlanMoves(root, []scanner.File{f(a, 1), f(b, 1)}, map[string]bool{b: true})
	if len(moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(moves))
	}
	if moves[0].Source != a {
		t.Errorf("expected only %s proposed, got %s", a, moves[0].Source)
	}
}

func TestPlanMovesDetectsConflicts(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "report.pdf")
	destDir := filepath.Join(root, "Documents")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(destDir, "report.pdf")
	if err := os.WriteFile(existing, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	moves := PlanMoves(root, []scanner.File{f(src, 1)}, nil)
	if len(moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(moves))
	}
	if !moves[0].Conflict {
		t.Error("expected move to be marked as conflict")
	}
	if !moves[0].Blocked() {
		t.Error("expected Blocked() to report true for a conflict")
	}
	// The existing destination must be untouched by planning.
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "precious" {
		t.Errorf("destination content changed: %q err=%v", got, err)
	}
}

func TestPlanMovesNeverIncludesNestedFiles(t *testing.T) {
	root := t.TempDir()
	files := []scanner.File{
		f(filepath.Join(root, "top.txt"), 1),
		f(filepath.Join(root, "a", "nested.txt"), 2),
		f(filepath.Join(root, "a", "b", "deep.txt"), 3),
	}
	moves := PlanMoves(root, files, nil)
	if len(moves) != 1 {
		t.Fatalf("expected only the top-level file proposed, got %d moves", len(moves))
	}
	if filepath.Base(moves[0].Source) != "top.txt" {
		t.Errorf("proposed source = %q, want top.txt", moves[0].Source)
	}
}
