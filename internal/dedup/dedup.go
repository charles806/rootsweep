// Package dedup finds exact duplicate files by grouping on size and hashing
// only the files that could possibly be duplicates.
package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"sort"
	"sync"

	"rootsweep/internal/scanner"
)

// Progress reports how far hashing has advanced.
type Progress struct {
	Done  int64 // files hashed so far
	Total int64 // files that need hashing
}

// Group is a set of files with byte-identical content.
type Group struct {
	Hash  string   // hex-encoded SHA-256 of the shared content
	Size  int64    // size of each file in bytes
	Files []string // sorted absolute paths
}

// ExtraCopies returns how many redundant copies the group holds.
func (g Group) ExtraCopies() int { return len(g.Files) - 1 }

// ExtraBytes returns the disk space occupied by the redundant copies.
func (g Group) ExtraBytes() int64 { return g.Size * int64(len(g.Files)-1) }

// Report is the outcome of duplicate detection.
type Report struct {
	Groups      []Group
	HashedFiles int64
	HashErrors  []error // files that could not be hashed
}

// ExtraCopyCount is the total number of redundant copies across all groups.
func (r *Report) ExtraCopyCount() int {
	n := 0
	for _, g := range r.Groups {
		n += g.ExtraCopies()
	}
	return n
}

// ExtraBytes is the total disk space occupied by redundant copies.
func (r *Report) ExtraBytes() int64 {
	var n int64
	for _, g := range r.Groups {
		n += g.ExtraBytes()
	}
	return n
}

// DupPaths returns the set of all paths that belong to any duplicate group.
func (r *Report) DupPaths() map[string]bool {
	set := make(map[string]bool)
	for _, g := range r.Groups {
		for _, p := range g.Files {
			set[p] = true
		}
	}
	return set
}

// Hashable returns the files whose size is shared with at least one other
// file — the only files that ever need to be hashed.
func Hashable(files []scanner.File) []scanner.File {
	bySize := make(map[int64]int)
	for _, f := range files {
		bySize[f.Size]++
	}
	var out []scanner.File
	for _, f := range files {
		if bySize[f.Size] > 1 {
			out = append(out, f)
		}
	}
	return out
}

type hashResult struct {
	path string
	size int64
	sum  string
	err  error
}

// FindDuplicates detects exact duplicate content among files.
//
// Files are first grouped by size; only groups with two or more members are
// hashed, using at most workers concurrent hashing goroutines. onProgress,
// when non-nil, is called as each file finishes hashing (and once up front
// with Done = 0). Files that cannot be hashed are recorded in Report.HashErrors.
func FindDuplicates(files []scanner.File, workers int, onProgress func(Progress)) *Report {
	if workers < 1 {
		workers = 1
	}
	report := &Report{}

	candidates := Hashable(files)
	report.HashedFiles = int64(len(candidates))
	if len(candidates) == 0 {
		return report
	}
	if onProgress != nil {
		onProgress(Progress{Done: 0, Total: int64(len(candidates))})
	}

	jobs := make(chan scanner.File)
	results := make(chan hashResult, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := sha256.New()
			buf := make([]byte, 32*1024)
			for f := range jobs {
				sum, err := hashFile(f.Path, h, buf)
				results <- hashResult{path: f.Path, size: f.Size, sum: sum, err: err}
			}
		}()
	}
	go func() {
		for _, f := range candidates {
			jobs <- f
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	byHash := make(map[string]*Group)
	var done int64
	for res := range results {
		done++
		if res.err != nil {
			report.HashErrors = append(report.HashErrors, fmt.Errorf("%s: %w", res.path, res.err))
		} else if g := byHash[res.sum]; g != nil {
			g.Files = append(g.Files, res.path)
		} else {
			byHash[res.sum] = &Group{Hash: res.sum, Size: res.size, Files: []string{res.path}}
		}
		if onProgress != nil {
			onProgress(Progress{Done: done, Total: int64(len(candidates))})
		}
	}

	for _, g := range byHash {
		if len(g.Files) > 1 {
			sort.Strings(g.Files)
			report.Groups = append(report.Groups, *g)
		}
	}
	// Largest wasted space first, then by first path for stability.
	sort.Slice(report.Groups, func(i, j int) bool {
		if report.Groups[i].ExtraBytes() != report.Groups[j].ExtraBytes() {
			return report.Groups[i].ExtraBytes() > report.Groups[j].ExtraBytes()
		}
		return report.Groups[i].Files[0] < report.Groups[j].Files[0]
	})
	return report
}

// hashFile computes the SHA-256 of a single file, streaming it in 32 KiB
// chunks. h and buf are reused by the caller; h is reset here.
func hashFile(path string, h hash.Hash, buf []byte) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h.Reset()
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
