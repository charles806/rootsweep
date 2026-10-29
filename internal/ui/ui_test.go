package ui

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbletea/v2"

	"rootsweep/internal/dedup"
	"rootsweep/internal/executor"
	"rootsweep/internal/organizer"
	"rootsweep/internal/scanner"
)

// key builds a KeyPressMsg for a named key the way the terminal would.
func key(s string) tea.KeyPressMsg {
	k := tea.Key{Text: s}
	switch s {
	case "enter":
		k = tea.Key{Code: tea.KeyEnter}
	case "up":
		k = tea.Key{Code: tea.KeyUp}
	case "down":
		k = tea.Key{Code: tea.KeyDown}
	case " ":
		k = tea.Key{Code: tea.KeySpace, Text: " "}
	}
	return tea.KeyPressMsg(k)
}

func movesFixture() []organizer.Move {
	return []organizer.Move{
		{Source: "/root/a.pdf", Dest: "/root/Documents/a.pdf", Category: organizer.Documents, Size: 100},
		{Source: "/root/b.jpg", Dest: "/root/Images/b.jpg", Category: organizer.Images, Size: 200},
		{Source: "/root/c.zip", Dest: "/root/Archives/c.zip", Category: organizer.Archives, Size: 300, Conflict: true},
	}
}

// newReadyModel returns a model in phaseReady with fixture moves loaded,
// bypassing the real scan so key handling can be tested in isolation.
func newReadyModel(t *testing.T, root string) *model {
	t.Helper()
	m := NewModel(root)
	m.phase = phaseReady
	m.files = nil
	m.report = &dedup.Report{}
	m.moves = movesFixture()
	m.selected = make([]bool, len(m.moves))
	return m
}

func TestCursorNavigationClamps(t *testing.T) {
	m := newReadyModel(t, "/root")

	m, _ = step(m, key("down"))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m, _ = step(m, key("down"))
	m, _ = step(m, key("down"))
	if m.cursor != 2 {
		t.Fatalf("cursor should clamp at last item, got %d", m.cursor)
	}
	m, _ = step(m, key("up"))
	m, _ = step(m, key("up"))
	m, _ = step(m, key("up"))
	if m.cursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", m.cursor)
	}
}

func TestToggleAndSelectAllSkipConflicts(t *testing.T) {
	m := newReadyModel(t, "/root")

	m, _ = step(m, key(" ")) // toggle first move
	if !m.selected[0] {
		t.Fatal("space should select the first move")
	}
	m, _ = step(m, key("down"))
	m, _ = step(m, key(" ")) // select second move
	if !m.selected[1] {
		t.Fatal("space should select the second move")
	}

	// The conflicting third move must be reported and left unselected.
	m, _ = step(m, key("down"))
	m, _ = step(m, key(" "))
	if m.selected[2] {
		t.Fatal("conflicting move must never be selectable")
	}
	if m.notice == "" {
		t.Fatal("expected a notice about the skipped destination")
	}

	// "a" selects every eligible move, skipping conflicts.
	m, _ = step(m, key("a"))
	if !m.selected[0] || !m.selected[1] {
		t.Fatal("select all should select the eligible moves")
	}
	if m.selected[2] {
		t.Fatal("select all must skip the conflicting move")
	}
	if len(m.selectedMoves()) != 2 {
		t.Fatalf("selected moves = %d, want 2", len(m.selectedMoves()))
	}
}

func TestEnterWithoutSelectionShowsNotice(t *testing.T) {
	m := newReadyModel(t, "/root")
	m, cmd := step(m, key("enter"))
	if m.phase != phaseReady {
		t.Fatalf("phase = %s, want to stay in ready", m.phase)
	}
	if cmd != nil {
		t.Fatal("no command should be issued without a selection")
	}
	if m.notice == "" {
		t.Fatal("expected a notice explaining there is nothing to review")
	}
}

func TestConfirmThenCancelNeverExecutes(t *testing.T) {
	root := t.TempDir()
	m := newReadyModel(t, root)
	m, _ = step(m, key("a")) // select everything eligible

	m, cmd := step(m, key("enter"))
	if m.phase != phaseConfirm {
		t.Fatalf("phase = %s, want confirm", m.phase)
	}
	if cmd != nil {
		t.Fatal("entering confirm must not trigger work")
	}
	if len(m.pending) != 2 {
		t.Fatalf("pending moves = %d, want 2", len(m.pending))
	}

	// Cancel: back to ready, nothing executed.
	m, cmd = step(m, key("n"))
	if m.phase != phaseReady {
		t.Fatalf("phase after cancel = %s, want ready", m.phase)
	}
	if cmd != nil {
		t.Fatal("cancel must not trigger work")
	}
	if m.results != nil {
		t.Fatal("no moves may be executed on cancel")
	}
}

func TestQuitKeyQuitsWithoutExecuting(t *testing.T) {
	root := t.TempDir()
	m := newReadyModel(t, root)
	m, _ = step(m, key("a"))
	m, _ = step(m, key("enter"))
	if m.phase != phaseConfirm {
		t.Fatalf("phase = %s, want confirm", m.phase)
	}
	m, cmd := step(m, key("q"))
	if cmd == nil {
		t.Fatal("q should return the quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("q should produce a QuitMsg, got %T", msg)
	}
	if m.results != nil {
		t.Fatal("unapproved moves must never be applied")
	}
}

// TestConfirmedMovesExecute drives the whole pipeline through the model with a
// real temporary folder: scan → hash → plan → select → review → confirm.
func TestConfirmedMovesExecute(t *testing.T) {
	root := t.TempDir()
	writeUIFile(t, filepath.Join(root, "a.pdf"), "pdf-bytes")
	writeUIFile(t, filepath.Join(root, "b.jpg"), "jpeg-bytes")
	writeUIFile(t, filepath.Join(root, "c.zip"), "zip-bytes")
	// A pre-existing destination that must never be overwritten.
	writeUIFile(t, filepath.Join(root, "Archives", "c.zip"), "precious")
	// A nested file that must be left untouched.
	writeUIFile(t, filepath.Join(root, "sub", "nested.pdf"), "nested")
	// A duplicate pair (identical content, different names).
	writeUIFile(t, filepath.Join(root, "dup1.txt"), "same")
	writeUIFile(t, filepath.Join(root, "dup2.txt"), "same")

	m := NewModel(root)

	// Scan and hash for real, feeding results back as the runtime would.
	scanRes, err := scanner.Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	m, _ = step(m, scanDoneMsg{result: scanRes})
	if m.phase != phaseHash {
		t.Fatalf("phase = %s, want hash", m.phase)
	}
	report := dedup.FindDuplicates(m.files, m.hashWorkers, nil)
	m, cmd := step(m, hashDoneMsg{report: report})
	if m.phase != phaseReady {
		t.Fatalf("phase = %s, want ready", m.phase)
	}
	if cmd != nil {
		t.Fatal("planning should not issue commands")
	}

	// The duplicate pair must have been found.
	if len(report.Groups) != 1 || len(report.Groups[0].Files) != 2 {
		t.Fatalf("expected one 2-file duplicate group, got %+v", report.Groups)
	}

	// Move proposals: only direct, non-duplicate files.
	var proposed []string
	for _, mv := range m.moves {
		proposed = append(proposed, filepath.Base(mv.Source))
	}
	assertContains(t, proposed, "a.pdf")
	assertContains(t, proposed, "b.jpg")
	assertContains(t, proposed, "c.zip")
	assertNotContains(t, proposed, "nested.pdf")
	assertNotContains(t, proposed, "dup1.txt")
	assertNotContains(t, proposed, "dup2.txt")

	// The conflicting move must be marked, not silently dropped.
	var conflict *organizer.Move
	for i := range m.moves {
		if m.moves[i].Conflict {
			conflict = &m.moves[i]
		}
	}
	if conflict == nil || filepath.Base(conflict.Source) != "c.zip" {
		t.Fatalf("expected c.zip to be marked as conflict, got %+v", m.moves)
	}

	// Select all eligible moves, review, confirm. The conflicting move must
	// stay unselected, so it never reaches the confirmation summary.
	conflictIdx := -1
	for i, mv := range m.moves {
		if mv.Conflict {
			conflictIdx = i
		}
	}
	if conflictIdx < 0 {
		t.Fatal("expected a conflicting move among the proposals")
	}
	m, _ = step(m, key("a"))
	if m.selected[conflictIdx] {
		t.Fatal("select all must skip the conflicting move")
	}
	m, _ = step(m, key("enter"))
	if m.phase != phaseConfirm {
		t.Fatalf("phase = %s, want confirm", m.phase)
	}
	if len(m.pending) != 2 {
		t.Fatalf("pending = %d, want 2 (conflicting move skipped)", len(m.pending))
	}
	_, cmd = step(m, key("y"))
	if cmd == nil {
		t.Fatal("y on the confirm screen must start execution")
	}
	// The execute command is a Batch; run every contained command and feed
	// the resulting messages back until execution completes.
	queue := []tea.Cmd{cmd}
	var done *execDoneMsg
	for len(queue) > 0 && done == nil {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case nil:
		default:
			if d, ok := msg.(execDoneMsg); ok {
				done = &d
			}
			m, _ = step(m, msg)
		}
	}
	if done == nil {
		t.Fatal("execution did not complete")
	}
	m, _ = step(m, *done)
	if m.phase != phaseDone {
		t.Fatalf("phase = %s, want done", m.phase)
	}

	// Results: the 2 approved moves completed.
	var moved int
	for _, r := range m.results {
		if r.Status == executor.Moved {
			moved++
		}
	}
	if moved != 2 || len(m.results) != 2 {
		t.Fatalf("results = %d moved, want exactly 2 moved", len(m.results))
	}

	// The approved moves really happened.
	assertFileContent(t, filepath.Join(root, "Documents", "a.pdf"), "pdf-bytes")
	assertFileContent(t, filepath.Join(root, "Images", "b.jpg"), "jpeg-bytes")
	// The conflicting source was never moved and the destination was not
	// overwritten.
	assertFileContent(t, filepath.Join(root, "c.zip"), "zip-bytes")
	assertFileContent(t, filepath.Join(root, "Archives", "c.zip"), "precious")
	// Nested files and duplicate-group members stayed put.
	assertFileContent(t, filepath.Join(root, "sub", "nested.pdf"), "nested")
	assertFileContent(t, filepath.Join(root, "dup1.txt"), "same")
	assertFileContent(t, filepath.Join(root, "dup2.txt"), "same")
}

// step feeds a message into the model and keeps the concrete type, mirroring
// what the Bubble Tea runtime does with the returned model.
func step(m *model, msg tea.Msg) (*model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(*model), cmd
}

// Fixture helpers.

func writeUIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s content = %q, want %q", path, got, want)
	}
}

func assertContains(t *testing.T, list []string, want string) {
	t.Helper()
	for _, s := range list {
		if s == want {
			return
		}
	}
	t.Errorf("expected %q in %v", want, list)
}

func assertNotContains(t *testing.T, list []string, unwanted string) {
	t.Helper()
	for _, s := range list {
		if s == unwanted {
			t.Errorf("did not expect %q in %v", unwanted, list)
		}
	}
}
