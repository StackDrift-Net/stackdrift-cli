//go:build !windows

package commands

import (
	"fmt"
	"os"
)

// replaceRunning puts the downloaded binary in place of the one executing this
// code.
//
// Renaming over a running executable is safe on Unix: the rename only unlinks
// the directory entry, and the kernel keeps the running image alive through the
// open file it was loaded from. Writing INTO the file would fail with ETXTBSY,
// which is why the download lands in a temp file in the same directory first.
//
// The displaced binary is kept beside the new one rather than thrown away. It
// is the only way back if the replacement turns out not to run, and unattended
// there is nobody to fetch another copy: the binary that would do it is the one
// that was overwritten.
//
// The installed path is returned because the caller cannot work it out again
// afterwards. Moving the running image makes /proc/self/exe, and therefore
// os.Executable, resolve to the displaced copy, so anything re-deriving the
// path would run the build that was just replaced.
func replaceRunning(newPath string) (string, error) {
	exe, err := currentExecutable()
	if err != nil {
		return "", err
	}

	if err := os.Chmod(newPath, 0o755); err != nil {
		return "", err
	}

	previous := previousPath(exe)
	kept := os.Rename(exe, previous) == nil

	if err := os.Rename(newPath, exe); err != nil {
		if kept {
			// Put the old one back rather than leave the machine with no
			// binary at all where its name is supposed to be.
			_ = os.Rename(previous, exe)
		}
		return "", fmt.Errorf("could not replace %s: %w", exe, err)
	}
	return exe, nil
}
