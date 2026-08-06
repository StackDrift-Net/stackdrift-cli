package commands

import (
	"fmt"
	"os"
)

// replaceRunning puts the downloaded binary in place of the one executing this
// code.
//
// Windows will not let a running image be overwritten, but it will let it be
// renamed, so the live binary is moved aside first. The move aside is also the
// way back if the new one cannot be put in place.
//
// The displaced copy is deliberately left on disk. os.Rename is MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, which has to delete the destination first, so the
// stale copy is cleared by the next update rather than by this process, which
// still holds the image mapped and could not delete it.
func replaceRunning(newPath string) (string, error) {
	exe, err := currentExecutable()
	if err != nil {
		return "", err
	}

	previous := previousPath(exe)
	os.Remove(previous)
	// Builds before the rollback copy was renamed left this behind, and nothing
	// clears it now that the name has changed
	os.Remove(exe + ".old")

	if err := os.Rename(exe, previous); err != nil {
		return "", fmt.Errorf("could not replace %s: %w", exe, err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(previous, exe)
		return "", fmt.Errorf("could not replace %s: %w", exe, err)
	}
	return exe, nil
}
