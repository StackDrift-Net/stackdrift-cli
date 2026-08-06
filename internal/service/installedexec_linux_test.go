package service

import (
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// Reading the path back out is what stops a second copy of the CLI repointing
// the scheduler at itself when all it was asked to do was flip a setting.

func TestExecFromUnit_ReadsThePathTheUnitStarts(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/home/u/.local/bin/stackdrift"}, false)

	if got := execFromUnit(unit); got != "/home/u/.local/bin/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

// systemd needs the path quoted when it holds a space, so reading it back has
// to take the quotes off again or the next install writes them twice.
func TestExecFromUnit_QuotedPath_ComesBackUnquoted(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/opt/my tools/stackdrift"}, false)

	if got := execFromUnit(unit); got != "/opt/my tools/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromUnit_ResidentUnit_ReadsThePathNotTheFlags(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalRealtime, Exec: "/x/stackdrift"}, true)

	if got := execFromUnit(unit); got != "/x/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

func TestExecFromUnit_NothingToRead_IsEmpty(t *testing.T) {
	if got := execFromUnit("[Service]\nType=oneshot\n"); got != "" {
		t.Fatalf("expected nothing, got %q", got)
	}
}

// The path is written back out as-is, so a read that truncates it leaves the
// scheduler starting something that does not exist.
func TestExecFromUnit_PathHoldingAQuote_RoundTrips(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: `/opt/a"b/stackdrift`}, false)

	if got := execFromUnit(unit); got != `/opt/a"b/stackdrift` {
		t.Fatalf("got %q", got)
	}
}
