package commands

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

// The roots are passed in rather than read from the environment so these run on
// every platform, which is the whole point: the machine that decides this is a
// Windows one and the machine the suite runs on is not.
var programRootsFixture = []string{`C:\Program Files`, `C:\Program Files (x86)`}

func TestSystemInstallDir_TheRootItself_IsSystem(t *testing.T) {
	if !systemInstallDir(`C:\Program Files`, programRootsFixture) {
		t.Fatal("expected Program Files itself to count")
	}
}

func TestSystemInstallDir_TheInstallDirectory_IsSystem(t *testing.T) {
	if !systemInstallDir(`C:\Program Files\StackDrift`, programRootsFixture) {
		t.Fatal("expected the install directory to count")
	}
}

// Windows paths are case-insensitive and nothing guarantees the casing the
// scheduler or os.Executable hands back matches the casing of the environment
// variable.
func TestSystemInstallDir_DifferentCasing_IsSystem(t *testing.T) {
	if !systemInstallDir(`c:\PROGRAM FILES\StackDrift`, programRootsFixture) {
		t.Fatal("expected the comparison to ignore case")
	}
}

func TestSystemInstallDir_TheThirtyTwoBitRoot_IsSystem(t *testing.T) {
	if !systemInstallDir(`C:\Program Files (x86)\StackDrift`, programRootsFixture) {
		t.Fatal("expected the x86 root to count")
	}
}

// The prefix has to end on a separator. Without that check a directory somebody
// created next door would be read as living inside the protected one, and its
// updates would be skipped for ever with a message about running an installer.
func TestSystemInstallDir_SiblingSharingThePrefix_IsNotSystem(t *testing.T) {
	if systemInstallDir(`C:\Program Files Extra\StackDrift`, programRootsFixture) {
		t.Fatal("a sibling directory must not be read as being inside the root")
	}
}

func TestSystemInstallDir_TrailingSeparatorOnTheRoot_IsSystem(t *testing.T) {
	if !systemInstallDir(`C:\Program Files\StackDrift`, []string{`C:\Program Files\`}) {
		t.Fatal("a root with a trailing separator must still match")
	}
}

func TestSystemInstallDir_UserProfileDirectory_IsNotSystem(t *testing.T) {
	if systemInstallDir(`C:\Users\Someone\AppData\Local\StackDrift`, programRootsFixture) {
		t.Fatal("the per-user install directory is writable and must not count")
	}
}

// Every platform but Windows supplies no roots at all, so nothing must ever be
// classified as a managed install there.
func TestSystemInstallDir_NoRoots_IsNotSystem(t *testing.T) {
	if systemInstallDir(`/usr/local/bin`, nil) {
		t.Fatal("with no roots nothing can be a system install")
	}
}

func TestSystemInstallDir_EmptyRoot_IsNotSystem(t *testing.T) {
	if systemInstallDir(`C:\Program Files\StackDrift`, []string{""}) {
		t.Fatal("an unset environment variable must not match everything")
	}
}

// Program Files refusing a write is the arrangement working, not a fault. It is
// where the installer puts the binary precisely because ordinary code cannot
// write there, so recording it as unreplaceable would turn the update check off
// on every Windows machine and leave them silently stale.
func TestClassifyUnreplaceable_PermissionInProgramFiles_NeedsTheInstaller(t *testing.T) {
	got := classifyUnreplaceable(`C:\Program Files\StackDrift`, programRootsFixture, fs.ErrPermission)
	if got != updateNeedsInstaller {
		t.Fatalf("expected updateNeedsInstaller, got %v", got)
	}
}

func TestClassifyUnreplaceable_PermissionElsewhere_IsBlocked(t *testing.T) {
	got := classifyUnreplaceable("/opt/stackdrift", programRootsFixture, fs.ErrPermission)
	if got != updateBlocked {
		t.Fatalf("expected updateBlocked, got %v", got)
	}
}

// A full disk under Program Files is not a rights problem and must not be
// answered with "run the installer as an administrator", which would not fix it.
func TestClassifyUnreplaceable_FullDiskInProgramFiles_IsDeferred(t *testing.T) {
	got := classifyUnreplaceable(`C:\Program Files\StackDrift`, programRootsFixture, syscall.ENOSPC)
	if got != updateDeferred {
		t.Fatalf("expected updateDeferred, got %v", got)
	}
}

func TestClassifyUnreplaceable_TransientFailure_IsDeferred(t *testing.T) {
	got := classifyUnreplaceable("/home/someone/bin", programRootsFixture, errors.New("resource temporarily unavailable"))
	if got != updateDeferred {
		t.Fatalf("expected updateDeferred, got %v", got)
	}
}
