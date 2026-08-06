package commands

import (
	"errors"
	"syscall"
)

// Windows never returns EROFS. Go defines the constant, but from its own
// invented range, so testing for it here would be a branch nothing can reach.
// A write-protected volume reports ERROR_WRITE_PROTECT instead.
const errorWriteProtect = syscall.Errno(19)

func readOnlyVolume(err error) bool {
	return errors.Is(err, errorWriteProtect)
}
