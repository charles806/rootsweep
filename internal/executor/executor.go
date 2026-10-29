// Package executor applies approved moves and reports exactly what happened.
package executor

import (
	"errors"
	"fmt"
	"os"

	"rootsweep/internal/organizer"
	"rootsweep/internal/safeio"
)

// Status describes what happened to one approved move.
type Status string

const (
	// Moved means the file now lives at its destination.
	Moved Status = "moved"
	// Blocked means the destination existed at move time and was left intact.
	Blocked Status = "blocked"
	// Failed means the move did not complete; the file is preserved.
	Failed Status = "failed"
)

// Result is the outcome of one executed move.
type Result struct {
	Move   organizer.Move
	Status Status
	Err    error // nil only when Status == Moved
}

func Execute(root string, moves []organizer.Move, onProgress func(done, total int)) []Result {
	results := make([]Result, 0, len(moves))
	for i, m := range moves {
		results = append(results, executeOne(root, m))
		if onProgress != nil {
			onProgress(i+1, len(moves))
		}
	}
	return results
}

func executeOne(root string, m organizer.Move) Result {
	if !safeio.Within(root, m.Dest) {
		return Result{Move: m, Status: Failed, Err: fmt.Errorf("%w: %s", safeio.ErrOutsideRoot, m.Dest)}
	}

	if _, err := os.Lstat(m.Dest); err == nil {
		return Result{Move: m, Status: Blocked, Err: fmt.Errorf("%w: %s", safeio.ErrDestinationExists, m.Dest)}
	}
	err := safeio.MoveNoOverwrite(m.Source, m.Dest)
	switch {
	case err == nil:
		return Result{Move: m, Status: Moved}
	case errors.Is(err, safeio.ErrDestinationExists):
		return Result{Move: m, Status: Blocked, Err: err}
	default:
		return Result{Move: m, Status: Failed, Err: err}
	}
}
