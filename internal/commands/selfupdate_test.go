package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

var checkedAt = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

func TestDueForUpdateCheck_NeverChecked_IsDue(t *testing.T) {
	if !dueForUpdateCheck("", checkedAt) {
		t.Fatal("a machine that has never checked must check")
	}
}

func TestDueForUpdateCheck_CheckedAnHourAgo_IsNotDue(t *testing.T) {
	stamp := checkedAt.Add(-time.Hour).Format(time.RFC3339)
	if dueForUpdateCheck(stamp, checkedAt) {
		t.Fatal("an install on a short interval must not call GitHub every sweep")
	}
}

func TestDueForUpdateCheck_CheckedLongerAgoThanTheFloor_IsDue(t *testing.T) {
	stamp := checkedAt.Add(-(updateCheckFloor + time.Minute)).Format(time.RFC3339)
	if !dueForUpdateCheck(stamp, checkedAt) {
		t.Fatal("expected a check once the floor has passed")
	}
}

// A damaged stamp must not wedge the check off for ever, which is what reading
// it as "checked just now" would do.
func TestDueForUpdateCheck_UnreadableStamp_IsDue(t *testing.T) {
	if !dueForUpdateCheck("last tuesday", checkedAt) {
		t.Fatal("an unreadable stamp must fall back to checking")
	}
}

// A clock that went backwards, or a file copied from a machine in the future,
// would otherwise park the next check beyond any horizon.
func TestDueForUpdateCheck_StampInTheFuture_IsDue(t *testing.T) {
	stamp := checkedAt.Add(48 * time.Hour).Format(time.RFC3339)
	if !dueForUpdateCheck(stamp, checkedAt) {
		t.Fatal("a stamp ahead of the clock must not defer the check")
	}
}

func TestShouldReplace_BehindTheRelease_IsTrue(t *testing.T) {
	if !shouldReplace("0.1.44", "v0.1.45", "") {
		t.Fatal("expected a build behind the release to be replaced")
	}
}

func TestShouldReplace_AlreadyOnTheRelease_IsFalse(t *testing.T) {
	if shouldReplace("0.1.45", "v0.1.45", "") {
		t.Fatal("nothing to do")
	}
}

// The running process keeps reporting the old version until it restarts, so
// without this a machine that cannot restart itself would download the same
// release every single day for ever.
func TestShouldReplace_ThatVersionAlreadyWrittenOverTheBinary_IsFalse(t *testing.T) {
	if shouldReplace("0.1.44", "v0.1.45", "0.1.45") {
		t.Fatal("a version already written to disk must not be fetched again")
	}
}

func TestShouldReplace_DevBuildWithNothingWritten_IsTrue(t *testing.T) {
	if !shouldReplace("dev", "v0.1.45", "") {
		t.Fatal("a build with no version is always behind")
	}
}

// needsUpdate treats dev as permanently behind, so the recorded version is the
// only thing that stops a dev build looping.
func TestShouldReplace_DevBuildAlreadyWritten_IsFalse(t *testing.T) {
	if shouldReplace("dev", "v0.1.45", "v0.1.45") {
		t.Fatal("a dev build must not re-download what it already wrote")
	}
}

func TestExeDirWritable_WritableDirectory_IsAccepted(t *testing.T) {
	if err := exeDirWritable(t.TempDir()); err != nil {
		t.Fatalf("expected a writable directory accepted, got %v", err)
	}
}

// Read-only is answered by trying, not by reading permission bits. Under
// systemd the thing that refuses the write is a read-only mount, which no
// permission bit shows.
func TestExeDirWritable_ReadOnlyDirectory_IsRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only mode bit")
	}

	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}

	if err := exeDirWritable(dir); err == nil {
		t.Fatal("expected a directory that cannot be written to be refused")
	}
}

func TestExeDirWritable_Always_LeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	if err := exeDirWritable(dir); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the probe must clean up after itself, found %d entries", len(entries))
	}
}

// A person typing "stackdrift watch" has not agreed to have their binary
// replaced. Only the run the scheduler starts carries that agreement.
func TestAutoUpdateWanted_TypedByHand_IsFalse(t *testing.T) {
	yes := true
	settings := &config.WatchSettings{AutoUpdate: &yes}

	if autoUpdateWanted(false, settings, false) {
		t.Fatal("a foreground run must never replace the binary")
	}
}

func TestAutoUpdateWanted_ScheduledAndAgreed_IsTrue(t *testing.T) {
	yes := true
	if !autoUpdateWanted(true, &config.WatchSettings{AutoUpdate: &yes}, false) {
		t.Fatal("expected the scheduled run to check")
	}
}

func TestAutoUpdateWanted_ScheduledButNeverAgreed_IsFalse(t *testing.T) {
	if autoUpdateWanted(true, &config.WatchSettings{Asked: true, Enabled: true}, false) {
		t.Fatal("an install that predates the question must stay as it is")
	}
}

func TestAutoUpdateWanted_ScheduledAndDeclined_IsFalse(t *testing.T) {
	no := false
	if autoUpdateWanted(true, &config.WatchSettings{AutoUpdate: &no}, false) {
		t.Fatal("expected a declined machine left alone")
	}
}

// The re-run after an update must not update again, or a build the release feed
// keeps reporting as behind would loop for ever.
func TestAutoUpdateWanted_TheRerunAfterAnUpdate_IsFalse(t *testing.T) {
	yes := true
	if autoUpdateWanted(true, &config.WatchSettings{AutoUpdate: &yes}, true) {
		t.Fatal("the child of an update must not update")
	}
}

// A binary that cannot be replaced would otherwise call GitHub and fail on
// every run for the life of the install.
func TestAutoUpdateWanted_RecordedAsUnreplaceable_IsFalse(t *testing.T) {
	yes := true
	settings := &config.WatchSettings{AutoUpdate: &yes, UpdateBlocked: "read-only file system"}

	if autoUpdateWanted(true, settings, false) {
		t.Fatal("a known-unreplaceable binary must stop being checked")
	}
}
