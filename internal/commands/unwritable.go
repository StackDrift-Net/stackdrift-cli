package commands

import (
	"errors"
	"io/fs"
)

// permanentlyUnwritable reports whether a failed write is one that waiting will
// not fix.
//
// The difference matters because a permanent cause is remembered and stops the
// update being tried again, while a full disk, a busy filesystem or a directory
// that was briefly missing must not turn auto-update off for the life of the
// install with nothing to say why.
//
// Permission covers a root-owned install directory under a user service and is
// the same everywhere. What a read-only volume reports is not, so each platform
// answers that in its own file.
func permanentlyUnwritable(err error) bool {
	return errors.Is(err, fs.ErrPermission) || readOnlyVolume(err)
}
