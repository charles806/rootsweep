// Package ui implements the RootSweep interactive terminal dashboard.
package ui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"rootsweep/internal/dedup"
	"rootsweep/internal/executor"
	"rootsweep/internal/organizer"
	"rootsweep/internal/scanner"
)

// phase is the top-level state of the dashboard.
type phase int

const (
	phaseScan phase = iota
	phaseHash
	phaseReady
	phaseConfirm
	phaseExecute
	phaseDone
)

func (p phase) String() string {
	switch p {
	case phaseScan:
		return "scanning"
	case phaseHash:
		return "hashing"
	case phaseReady:
		return "ready"
	case phaseConfirm:
		return "confirm"
	case phaseExecute:
		return "executing"
	case phaseDone:
		return "done"
	}
	return "unknown"
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("237"))
	selectedSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	conflictSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	accentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	faintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 2)
	badgeScan    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("195"))
	badgeReady   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	badgeConfirm = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
)

func badgeFor(p phase) string {
	label := " " + p.String() + " "
	switch p {
	case phaseReady, phaseDone:
		return badgeReady.Render(label)
	case phaseConfirm:
		return badgeConfirm.Render(label)
	default:
		return badgeScan.Render(label)
	}
}

// Run starts the interactive dashboard and blocks until the program exits.
func Run(root string) error {
	p := tea.NewProgram(NewModel(root))
	_, err := p.Run()
	return err
}

type scanTickMsg struct{}
type scanDoneMsg struct {
	result *scanner.ScanResult
	err    error
}
type hashTickMsg struct{}
type hashDoneMsg struct {
	report *dedup.Report
}
type execTickMsg struct{}
type execDoneMsg struct {
	results []executor.Result
}

type model struct {
	root  string
	phase phase

	width  int
	height int

	// Scanning state.
	scanned    int
	scanErrors []error
	symlinks   int
	files      []scanner.File

	// Hashing state.
	report      *dedup.Report
	hashTotal   int
	hashed      int
	hashWorkers int

	// Planning state.
	moves    []organizer.Move
	selected []bool
	cursor   int
	notice   string

	// Confirmation / execution state.
	pending   []organizer.Move
	results   []executor.Result
	execTotal int
	execCount int

	quitting bool

	scanProgress chan struct{}
	hashProgress chan struct{}
	execProgress chan struct{}
}

// NewModel creates the dashboard model for the given root folder.
func NewModel(root string) *model {
	return &model{
		root:         root,
		phase:        phaseScan,
		scanProgress: make(chan struct{}, 256),
		hashProgress: make(chan struct{}, 256),
		hashWorkers:  hashWorkerCount(),
	}
}

func hashWorkerCount() int {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	return n
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.runScan(), m.scanTick())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case scanTickMsg:
		if m.phase != phaseScan {
			return m, nil
		}
		m.scanned++
		return m, m.scanTick()

	case scanDoneMsg:
		return m.handleScanDone(msg)

	case hashTickMsg:
		if m.phase != phaseHash {
			return m, nil
		}
		m.hashed++
		return m, m.hashTick()

	case hashDoneMsg:
		return m.handleHashDone(msg)

	case execTickMsg:
		if m.phase != phaseExecute {
			return m, nil
		}
		m.execCount++
		return m, m.execTick()

	case execDoneMsg:
		return m.handleExecDone(msg)
	}
	return m, nil
}

// Commands driving the background work. Progress updates flow back through
// channels closed when the work finishes, which keeps the UI responsive.

func (m *model) runScan() tea.Cmd {
	return func() tea.Msg {
		result, err := scanner.Scan(m.root, func(scanner.File) {
			select {
			case m.scanProgress <- struct{}{}:
			default:
			}
		})
		close(m.scanProgress)
		return scanDoneMsg{result: result, err: err}
	}
}

func (m *model) scanTick() tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-m.scanProgress; !ok {
			return nil
		}
		return scanTickMsg{}
	}
}

func (m *model) handleScanDone(msg scanDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.scanErrors = append(m.scanErrors, msg.err)
	}
	if msg.result != nil {
		m.files = msg.result.Files
		m.scanErrors = append(m.scanErrors, msg.result.ScanErrors...)
		m.symlinks = msg.result.SymlinksSkipped
	}
	m.phase = phaseHash
	m.hashTotal = len(dedup.Hashable(m.files))
	if m.hashTotal == 0 {
		// Nothing can be a duplicate; skip hashing entirely.
		if msg.err != nil {
			m.notice = "scan failed: " + msg.err.Error()
		}
		return m.handleHashDone(hashDoneMsg{report: &dedup.Report{}})
	}
	return m, tea.Batch(m.runHash(), m.hashTick())
}

func (m *model) runHash() tea.Cmd {
	return func() tea.Msg {
		report := dedup.FindDuplicates(m.files, m.hashWorkers, func(dedup.Progress) {
			select {
			case m.hashProgress <- struct{}{}:
			default:
			}
		})
		close(m.hashProgress)
		return hashDoneMsg{report: report}
	}
}

func (m *model) hashTick() tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-m.hashProgress; !ok {
			return nil
		}
		return hashTickMsg{}
	}
}

func (m *model) handleHashDone(msg hashDoneMsg) (tea.Model, tea.Cmd) {
	m.report = msg.report
	m.moves = organizer.PlanMoves(m.root, m.files, msg.report.DupPaths())
	m.selected = make([]bool, len(m.moves))
	m.cursor = 0
	m.phase = phaseReady
	if len(m.moves) == 0 {
		m.notice = "no eligible files to organize"
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	switch m.phase {
	case phaseReady:
		switch msg.String() {
		case "up":
			m.moveCursor(-1)
		case "down":
			m.moveCursor(1)
		case "space", " ":
			// ultraviolet reports a real spacebar press as "space".
			m.toggle()
		case "a":
			m.selectAll()
		case "enter":
			return m.openConfirm()
		}
	case phaseConfirm:
		switch msg.String() {
		case "n":
			m.phase = phaseReady
		case "y":
			return m.startExecute()
		}
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	if len(m.moves) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.moves) {
		m.cursor = len(m.moves) - 1
	}
	m.notice = ""
}

func (m *model) toggle() {
	if m.cursor >= len(m.moves) {
		return
	}
	if m.moves[m.cursor].Conflict {
		m.notice = "destination exists — move skipped"
		return
	}
	m.selected[m.cursor] = !m.selected[m.cursor]
	m.notice = ""
}

func (m *model) selectAll() {
	for i, mv := range m.moves {
		m.selected[i] = !mv.Conflict
	}
	m.notice = ""
}

func (m *model) selectedMoves() []organizer.Move {
	var out []organizer.Move
	for i, sel := range m.selected {
		if sel {
			out = append(out, m.moves[i])
		}
	}
	return out
}

func (m *model) openConfirm() (tea.Model, tea.Cmd) {
	m.pending = m.selectedMoves()
	if len(m.pending) == 0 {
		m.notice = "no eligible moves selected"
		return m, nil
	}
	m.phase = phaseConfirm
	m.notice = ""
	return m, nil
}

func (m *model) startExecute() (tea.Model, tea.Cmd) {
	m.phase = phaseExecute
	m.execTotal = len(m.pending)
	m.execCount = 0
	m.execProgress = make(chan struct{}, 256)
	m.notice = ""
	return m, tea.Batch(m.runExecute(), m.execTick())
}

func (m *model) runExecute() tea.Cmd {
	return func() tea.Msg {
		results := executor.Execute(m.root, m.pending, func(done, total int) {
			select {
			case m.execProgress <- struct{}{}:
			default:
			}
		})
		close(m.execProgress)
		return execDoneMsg{results: results}
	}
}

func (m *model) execTick() tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-m.execProgress; !ok {
			return nil
		}
		return execTickMsg{}
	}
}

func (m *model) handleExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	m.results = msg.results
	m.phase = phaseDone
	return m, nil
}

// View renders the dashboard for the current phase.

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *model) render() string {
	w, h := m.viewSize()
	var b strings.Builder
	b.WriteString(m.header(w))
	switch m.phase {
	case phaseScan:
		b.WriteString(m.scanView())
	case phaseHash:
		b.WriteString(m.hashView(w))
	case phaseReady:
		b.WriteString(m.readyView(w, h))
	case phaseConfirm:
		b.WriteString(m.confirmView(w))
	case phaseExecute:
		b.WriteString(m.executeView(w))
	case phaseDone:
		b.WriteString(m.doneView(w))
	}
	b.WriteString(m.footer(w))
	return capLines(truncateLines(b.String(), w), h)
}

func (m *model) viewSize() (w, h int) {
	w, h = m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return w, h
}

func (m *model) header(w int) string {
	line := lipgloss.JoinHorizontal(lipgloss.Left,
		titleStyle.Render("RootSweep"),
		mutedStyle.Render(m.root),
		badgeFor(m.phase),
	)
	return line + "\n" + faintStyle.Render(strings.Repeat("─", w)) + "\n"
}

func (m *model) footer(w int) string {
	var hints string
	switch m.phase {
	case phaseReady:
		hints = "↑/↓ navigate · space select · a select all · enter review · q quit"
	case phaseConfirm:
		hints = "y confirm · n cancel · q quit"
	case phaseDone:
		hints = "q quit"
	default:
		hints = "q quit"
	}
	return "\n" + footerStyle.Render(hints) + "\n"
}

func (m *model) scanView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  Scanning %s…\n\n", m.root)
	fmt.Fprintf(&b, "  files found: %s\n", mutedStyle.Render(fmt.Sprint(m.scanned)))
	if m.symlinks > 0 {
		fmt.Fprintf(&b, "  symlinks skipped: %d\n", m.symlinks)
	}
	if len(m.scanErrors) > 0 {
		fmt.Fprintf(&b, "  %s %d unreadable path(s) skipped\n", warnStyle.Render("warning:"), len(m.scanErrors))
	}
	return b.String()
}

func (m *model) hashView(w int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  Detecting duplicates with SHA-256…\n\n")
	pct := 0.0
	if m.hashTotal > 0 {
		pct = float64(m.hashed) / float64(m.hashTotal)
	}
	fmt.Fprintf(&b, "  %s %s\n", bar(pct, min(w-6, 40)), mutedStyle.Render(fmt.Sprintf("%d/%d files hashed", m.hashed, m.hashTotal)))
	return b.String()
}

func (m *model) readyView(w, h int) string {
	var b strings.Builder

	direct := 0
	for _, f := range m.files {
		if f.Depth == 1 {
			direct++
		}
	}
	fmt.Fprintf(&b, "\n  Scanned %s (%s at root)", mutedStyle.Render(fmt.Sprint(len(m.files))), mutedStyle.Render(fmt.Sprint(direct)))
	if m.symlinks > 0 {
		fmt.Fprintf(&b, ", %d symlink(s) skipped", m.symlinks)
	}
	b.WriteString("\n")
	if len(m.scanErrors) > 0 {
		fmt.Fprintf(&b, "  %s %d unreadable path(s) skipped\n", warnStyle.Render("warning:"), len(m.scanErrors))
	}

	// Duplicate groups section.
	dupHeader := fmt.Sprintf("Duplicates: %d group(s) · %s reclaimable",
		len(m.report.Groups), fmtBytes(m.report.ExtraBytes()))
	fmt.Fprintf(&b, "\n  %s\n", headerStyle.Render(dupHeader))
	used := 6 // header + rule + stats lines so far (approximate)
	if len(m.report.Groups) == 0 {
		fmt.Fprintf(&b, "  No duplicate files found.\n")
		used++
	} else {
		shown := 0
		for i, g := range m.report.Groups {
			if shown >= 5 {
				fmt.Fprintf(&b, "  … %d more group(s)\n", len(m.report.Groups)-shown)
				used++
				break
			}
			fmt.Fprintf(&b, "  %d. %s · %s\n", i+1, mutedStyle.Render(fmt.Sprintf("%d copies", len(g.Files))), fmtBytes(g.Size))
			used++
			for _, p := range g.Files {
				fmt.Fprintf(&b, "      %s\n", faintStyle.Render(clip(p, w-8)))
				used++
			}
			shown++
		}
	}

	// Proposed moves section.
	sel := 0
	for i, mv := range m.moves {
		if m.selected[i] && !mv.Conflict {
			sel++
		}
	}
	fmt.Fprintf(&b, "\n  %s (%s selected)\n", headerStyle.Render(fmt.Sprintf("Proposed moves: %d", len(m.moves))), mutedStyle.Render(fmt.Sprint(sel)))
	used += 2

	moveRows := h - used - 3 // footer + blank lines
	if moveRows < 3 {
		moveRows = 3
	}
	if moveRows > len(m.moves) {
		moveRows = len(m.moves)
	}
	start := 0
	if m.cursor >= moveRows {
		start = m.cursor - moveRows + 1
	}
	for i := start; i < start+moveRows && i < len(m.moves); i++ {
		b.WriteString(m.moveLine(i, w))
	}
	if len(m.moves) == 0 {
		fmt.Fprintf(&b, "  Nothing to move.\n")
	}
	if m.notice != "" {
		fmt.Fprintf(&b, "\n  %s\n", warnStyle.Render(m.notice))
	}
	return b.String()
}

func (m *model) moveLine(i, w int) string {
	mv := m.moves[i]
	checkbox := "[ ]"
	if m.selected[i] {
		checkbox = selectedSty.Render("[x]")
	}
	name := clip(filepath.Base(mv.Source), 24)
	var line string
	if mv.Conflict {
		line = fmt.Sprintf("%s %s  %s %s",
			checkbox, name, conflictSty.Render("→ exists"), mutedStyle.Render("skipped"))
	} else {
		dest := string(mv.Category) + "/" + name
		line = fmt.Sprintf("%s %s  %s %s  %s",
			checkbox, name, mutedStyle.Render("→"), clip(dest, max(w-44, 12)), mutedStyle.Render(fmtBytes(mv.Size)))
	}
	if i == m.cursor {
		return cursorStyle.Render("❯ "+line) + "\n"
	}
	return "  " + line + "\n"
}

func (m *model) confirmView(w int) string {
	var b strings.Builder
	var total int64
	for _, mv := range m.pending {
		total += mv.Size
	}
	fmt.Fprintf(&b, "\n  %s\n\n", headerStyle.Render(fmt.Sprintf("Review: %d move(s) · %s", len(m.pending), fmtBytes(total))))
	for _, mv := range m.pending {
		fmt.Fprintf(&b, "  %s  %s %s\n",
			clip(filepath.Base(mv.Source), 30),
			mutedStyle.Render("→"),
			clip(mv.Dest, max(w-40, 20)))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n", mutedStyle.Render("Existing destinations are never overwritten; conflicts are skipped."))
	fmt.Fprintf(&b, "\n  %s\n", accentStyle.Render("Press y to confirm, n to cancel."))
	return b.String()
}

func (m *model) executeView(w int) string {
	var b strings.Builder
	pct := 0.0
	if m.execTotal > 0 {
		pct = float64(m.execCount) / float64(m.execTotal)
	}
	fmt.Fprintf(&b, "\n  Executing approved moves…\n\n")
	fmt.Fprintf(&b, "  %s %s\n", bar(pct, min(w-6, 40)), mutedStyle.Render(fmt.Sprintf("%d/%d moves", m.execCount, m.execTotal)))
	return b.String()
}

func (m *model) doneView(w int) string {
	var b strings.Builder
	var moved, blocked, failed int
	for _, r := range m.results {
		switch r.Status {
		case executor.Moved:
			moved++
		case executor.Blocked:
			blocked++
		case executor.Failed:
			failed++
		}
	}
	fmt.Fprintf(&b, "\n  %s\n\n", headerStyle.Render(fmt.Sprintf("Done: %d moved · %d skipped · %d failed", moved, blocked, failed)))
	for _, r := range m.results {
		switch r.Status {
		case executor.Moved:
			fmt.Fprintf(&b, "  %s %s → %s\n", okStyle.Render("moved"), clip(r.Move.Source, 30), clip(r.Move.Dest, max(w-44, 20)))
		case executor.Blocked:
			fmt.Fprintf(&b, "  %s %s (destination exists)\n", warnStyle.Render("skipped"), clip(r.Move.Source, 30))
		case executor.Failed:
			fmt.Fprintf(&b, "  %s %s: %s\n", errStyle.Render("failed"), clip(r.Move.Source, 30), errStyle.Render(errString(r.Err)))
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n", mutedStyle.Render("Press q to quit."))
	return b.String()
}

func errString(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

// Helpers.

func bar(pct float64, width int) string {
	if width < 4 {
		return ""
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func fmtBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func clip(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func truncateLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if lipgloss.Width(ln) > w {
			lines[i] = ansi.Truncate(ln, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// capLines keeps a view from growing past the terminal height.
func capLines(s string, h int) string {
	if h <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}
