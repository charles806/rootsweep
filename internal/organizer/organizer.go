// Package organizer plans file-type-based moves for files at the root of a
// scanned folder. It never touches files and never detects conflicts by
// overwriting anything — it only proposes.
package organizer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rootsweep/internal/scanner"
)

// Category is a suggested destination folder name.
type Category string

const (
	Documents  Category = "Documents"
	Images     Category = "Images"
	Videos     Category = "Videos"
	Audio      Category = "Audio"
	Archives   Category = "Archives"
	Installers Category = "Installers"
	Code       Category = "Code"
	Other      Category = "Other"
)

// Categories lists every supported category in display order.
var Categories = []Category{Documents, Images, Videos, Audio, Archives, Installers, Code, Other}

var categoryExtensions = map[Category][]string{
	Documents: {".pdf", ".doc", ".docx", ".txt", ".md", ".rtf", ".odt", ".ods",
		".xls", ".xlsx", ".csv", ".ppt", ".pptx", ".pages", ".numbers", ".key",
		".tex", ".epub", ".mobi", ".msg", ".eml"},
	Images: {".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp", ".tiff",
		".tif", ".ico", ".heic", ".heif", ".raw", ".cr2", ".nef", ".arw", ".psd", ".avif"},
	Videos: {".mp4", ".mov", ".avi", ".mkv", ".wmv", ".flv", ".webm", ".m4v",
		".mpg", ".mpeg", ".3gp", ".mts", ".m2ts", ".vob", ".ogv"},
	Audio: {".mp3", ".wav", ".flac", ".aac", ".ogg", ".oga", ".m4a", ".wma",
		".aiff", ".aif", ".opus", ".mid", ".midi", ".mka"},
	Archives: {".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".zst",
		".iso", ".cab", ".lz", ".lzma", ".z", ".arj", ".ace"},
	Installers: {".dmg", ".pkg", ".mpkg"},
	Code: {".go", ".py", ".js", ".mjs", ".ts", ".tsx", ".jsx", ".java", ".c",
		".h", ".cpp", ".cc", ".hpp", ".cs", ".rb", ".php", ".rs", ".swift",
		".kt", ".kts", ".scala", ".sh", ".bash", ".zsh", ".fish", ".pl", ".pm",
		".lua", ".r", ".jl", ".m", ".sql", ".html", ".htm", ".css", ".scss",
		".sass", ".less", ".json", ".jsonl", ".yaml", ".yml", ".toml", ".xml",
		".ini", ".cfg", ".conf", ".env", ".gradle", ".vue", ".svelte", ".ex",
		".exs", ".erl", ".hrl", ".clj", ".cljs", ".fs", ".fsx", ".dart",
		".ipynb", ".proto", ".tf", ".dockerfile"},
}

// extCategory is built once from categoryExtensions for fast lookups.
var extCategory = func() map[string]Category {
	m := make(map[string]Category, 128)
	for cat, exts := range categoryExtensions {
		for _, e := range exts {
			m[e] = cat
		}
	}
	return m
}()

// CategoryFor maps a file name to a category based on its extension.
// Matching is case-insensitive; unmatched names fall back to Other.
func CategoryFor(name string) Category {
	ext := strings.ToLower(filepath.Ext(name))
	if cat, ok := extCategory[ext]; ok {
		return cat
	}
	return Other
}

// Move is one proposed file move.
type Move struct {
	Source   string   // current absolute path
	Dest     string   // proposed absolute destination path
	Category Category // destination category folder
	Size     int64    // source size in bytes
	Conflict bool     // destination already exists; move must be skipped
}

// Blocked reports whether the move cannot proceed.
func (m Move) Blocked() bool { return m.Conflict }

// PlanMoves proposes a category destination for every eligible file that
// sits directly inside root. Nested files are never proposed, and neither
// are members of duplicate groups. Destinations that already exist are
// marked as conflicts but are never overwritten or pre-empted.
func PlanMoves(root string, files []scanner.File, duplicatePaths map[string]bool) []Move {
	var moves []Move
	for _, f := range files {
		if f.Depth != 1 {
			continue // only files directly inside the root may be moved
		}
		if duplicatePaths[f.Path] {
			continue // duplicates stay put; deduplication is the user's decision
		}
		cat := CategoryFor(filepath.Base(f.Path))
		dest := filepath.Join(root, string(cat), filepath.Base(f.Path))
		m := Move{Source: f.Path, Dest: dest, Category: cat, Size: f.Size}
		if _, err := os.Lstat(dest); err == nil {
			m.Conflict = true
		}
		moves = append(moves, m)
	}
	sort.Slice(moves, func(i, j int) bool {
		if moves[i].Category != moves[j].Category {
			return moves[i].Category < moves[j].Category
		}
		return moves[i].Source < moves[j].Source
	})
	return moves
}
