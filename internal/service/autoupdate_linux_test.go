package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// The unit sandboxes the scan with ProtectSystem=strict and ProtectHome=read-only,
// which makes the directory holding the binary read-only too. Proved against real
// systemd: the replace fails with "Read-only file system" before a byte is
// downloaded. So opting in has to widen the sandbox by exactly one directory.
func TestUnitFile_AutoUpdateChosen_LetsTheUnitWriteTheBinaryDirectory(t *testing.T) {
	unit := unitFile(Plan{
		Interval:   config.IntervalDaily,
		Exec:       "/home/u/.local/bin/stackdrift",
		AutoUpdate: true,
	}, false)

	writable := readWritePaths(unit)
	if !strings.Contains(writable, "/home/u/.local/bin") {
		t.Fatalf("the unit cannot replace a binary in a directory it cannot write:\n%s", unit)
	}
	if !strings.Contains(writable, "%h/.stackdrift") {
		t.Fatalf("the link store must stay writable:\n%s", unit)
	}
}

// The path also appears in ExecStart, so only the sandbox line answers this.
func readWritePaths(unit string) string {
	for _, line := range strings.Split(unit, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "ReadWritePaths="); found {
			return rest
		}
	}
	return ""
}

// Anyone who said no keeps the tighter sandbox, so the scan never holds write
// access to a directory on their PATH.
func TestUnitFile_AutoUpdateDeclined_KeepsTheBinaryDirectoryReadOnly(t *testing.T) {
	unit := unitFile(Plan{
		Interval: config.IntervalDaily,
		Exec:     "/home/u/.local/bin/stackdrift",
	}, false)

	if strings.Contains(readWritePaths(unit), "/home/u/.local/bin") {
		t.Fatalf("nothing should have widened the sandbox:\n%s", unit)
	}
}

// A path that does not exist stops the unit starting at all unless it is
// prefixed, which would take the whole watcher down rather than just the update.
func TestUnitFile_AutoUpdateChosen_ToleratesAMissingDirectory(t *testing.T) {
	unit := unitFile(Plan{
		Interval:   config.IntervalDaily,
		Exec:       "/opt/sd/stackdrift",
		AutoUpdate: true,
	}, false)

	if !strings.Contains(readWritePaths(unit), "-/opt/sd") {
		t.Fatalf("expected the added path prefixed so a missing one is not fatal:\n%s", unit)
	}
}

// The resident unit replaces its binary and then exits for systemd to start
// again, so it needs the same access the timer unit does.
func TestUnitFile_ResidentWithAutoUpdate_CanAlsoWriteTheBinaryDirectory(t *testing.T) {
	unit := unitFile(Plan{
		Interval:   config.IntervalRealtime,
		Exec:       "/home/u/.local/bin/stackdrift",
		AutoUpdate: true,
	}, true)

	if !strings.Contains(readWritePaths(unit), "/home/u/.local/bin") {
		t.Fatalf("the resident unit needs the same access:\n%s", unit)
	}
}

// Nothing else distinguishes the run systemd starts from a person typing
// "stackdrift watch", and only the former has been agreed to replace a binary.
func TestUnitFile_Always_MarksTheRunAsScheduled(t *testing.T) {
	for _, resident := range []bool{false, true} {
		interval := config.IntervalDaily
		if resident {
			interval = config.IntervalRealtime
		}
		unit := unitFile(Plan{Interval: interval, Exec: "/x/stackdrift"}, resident)

		if !strings.Contains(unit, "watch --scheduled") && !strings.Contains(unit, "--resident --scheduled") {
			t.Fatalf("resident=%v: the scheduler's own run has to be identifiable:\n%s", resident, unit)
		}
	}
}

// systemd restarts the resident unit after any exit, so replacing the binary and
// standing down is how the new one takes over. Verified against real systemd: a
// Type=simple unit with Restart=always is restarted on a clean exit 0.
func TestRestartsOnExit_Systemd_IsTrue(t *testing.T) {
	if !RestartsOnExit() {
		t.Fatal("the resident unit sets Restart=always, so exiting is how it hands over")
	}
}
