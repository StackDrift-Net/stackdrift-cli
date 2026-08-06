package commands

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
)

// A root-owned install directory under a user service never becomes writable,
// so remembering it is what stops a pointless call on every run for ever.
func TestPermanentlyUnwritable_PermissionDenied_IsPermanent(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/usr/local/bin/x", Err: syscall.EACCES}

	if !permanentlyUnwritable(err) {
		t.Fatal("expected a permission failure treated as permanent")
	}
}

// The systemd sandbox, and a genuinely read-only mount, both land here.
func TestPermanentlyUnwritable_ReadOnlyFilesystem_IsPermanent(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/x", Err: syscall.EROFS}

	if !permanentlyUnwritable(err) {
		t.Fatal("expected a read-only filesystem treated as permanent")
	}
}

// A full disk clears itself. Latching on it would turn auto-update off for the
// life of the install with nothing to say why.
func TestPermanentlyUnwritable_DiskFull_IsNotPermanent(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/x", Err: syscall.ENOSPC}

	if permanentlyUnwritable(err) {
		t.Fatal("a full disk must not turn the feature off for good")
	}
}

func TestPermanentlyUnwritable_DirectoryMissing_IsNotPermanent(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}

	if permanentlyUnwritable(err) {
		t.Fatal("a directory that is briefly absent must not be final")
	}
}

func TestPermanentlyUnwritable_WrappedPermission_IsStillRecognised(t *testing.T) {
	err := errors.Join(errors.New("context"), fs.ErrPermission)

	if !permanentlyUnwritable(err) {
		t.Fatal("the cause has to be found through whatever wraps it")
	}
}
