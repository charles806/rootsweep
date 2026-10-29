// Command rootsweep launches the RootSweep interactive terminal file organizer.
//
// Usage: rootsweep <folder>
package main

import (
	"fmt"
	"os"

	"rootsweep/internal/scanner"
	"rootsweep/internal/ui"
)

const usage = `Usage: rootsweep <folder>

RootSweep scans a folder for duplicate files and proposes file-type based
moves. It is strictly read-only unless you explicitly confirm moves, and it
never deletes or overwrites files.`

func main() {
	if len(os.Args) != 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	root, err := scanner.ValidateRoot(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rootsweep: %v\n", err)
		os.Exit(1)
	}
	if err := ui.Run(root); err != nil {
		fmt.Fprintf(os.Stderr, "rootsweep: %v\n", err)
		os.Exit(1)
	}
}
