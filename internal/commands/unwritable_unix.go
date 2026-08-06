//go:build !windows

package commands

import (
	"errors"
	"syscall"
)

// EROFS is what a read-only mount reports, which is also what the systemd
// sandbox produces when the binary's directory is outside ReadWritePaths.
func readOnlyVolume(err error) bool {
	return errors.Is(err, syscall.EROFS)
}
