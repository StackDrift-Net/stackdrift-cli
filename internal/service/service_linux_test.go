package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestUnitFile_Realtime_StaysRunningAndRestarts(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalRealtime, Exec: "/usr/local/bin/stackdrift"}, true)

	if !strings.Contains(unit, "Type=simple") {
		t.Fatalf("a resident watcher is not a oneshot:\n%s", unit)
	}
	if !strings.Contains(unit, "watch --resident") {
		t.Fatalf("expected the resident flag passed:\n%s", unit)
	}
	if !strings.Contains(unit, "Restart=always") {
		t.Fatalf("a watcher that dies silently is worse than none:\n%s", unit)
	}
}

func TestUnitFile_Scheduled_RunsOnceAndExits(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalHourly, Exec: "/usr/local/bin/stackdrift"}, false)

	if !strings.Contains(unit, "Type=oneshot") {
		t.Fatalf("a timed sweep must exit so nothing is resident between runs:\n%s", unit)
	}
	if strings.Contains(unit, "--resident") {
		t.Fatalf("a timed sweep must not stay running:\n%s", unit)
	}
}

// Only a unit that starts itself carries an Install section. A timer-driven
// oneshot enabled directly would run once at boot and never again.
func TestUnitFile_Scheduled_IsNotEnabledOnItsOwn(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "[Install]") {
		t.Fatalf("the timer owns the schedule, not the service:\n%s", unit)
	}
}

func TestUnitFile_Realtime_IsEnabledOnItsOwn(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalRealtime, Exec: "/x/stackdrift"}, true)

	if !strings.Contains(unit, "[Install]") {
		t.Fatalf("a resident watcher has no timer to start it:\n%s", unit)
	}
}

func TestUnitFile_Always_YieldsToEverythingElse(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if !strings.Contains(unit, "Nice=10") || !strings.Contains(unit, "IOSchedulingClass=idle") {
		t.Fatalf("a background scan must not compete with real work:\n%s", unit)
	}
}

// The scan reads the machine and writes only its own link store, so anything
// more than that is authority it does not need.
func TestUnitFile_Always_CanOnlyWriteItsOwnStore(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if !strings.Contains(unit, "ProtectSystem=strict") {
		t.Fatalf("expected the filesystem protected:\n%s", unit)
	}
	if !strings.Contains(unit, "ReadWritePaths=%h/.stackdrift") {
		t.Fatalf("expected exactly one writable path:\n%s", unit)
	}
	if !strings.Contains(unit, "NoNewPrivileges=true") {
		t.Fatalf("expected privilege escalation refused:\n%s", unit)
	}
}

// A private /tmp would be a different /tmp from the one the person scanning
// saw, so a project path under it would be scanned as an empty directory and
// report nothing at all. The unit has to see the filesystem the scan did.
func TestUnitFile_Always_SharesTheRealTmp(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "PrivateTmp") {
		t.Fatalf("the unit must see the same /tmp the scan did:\n%s", unit)
	}
}

func TestUnitFile_Always_RecordsTheIntervalItWasBuiltFor(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalTwiceDay, Exec: "/x/stackdrift"}, false)

	if got := intervalFromUnit(unit); got != config.IntervalTwiceDay {
		t.Fatalf("status reads the interval back out of the unit, got %q", got)
	}
}

func TestIntervalFromUnit_NoMarker_IsEmpty(t *testing.T) {
	if got := intervalFromUnit("[Service]\nExecStart=/x/stackdrift watch\n"); got != "" {
		t.Fatalf("expected nothing for a unit with no marker, got %q", got)
	}
}

func TestTimerFile_Always_UsesTheIntervalSeconds(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalFiveMin})

	if !strings.Contains(timer, "OnUnitActiveSec=300s") {
		t.Fatalf("expected the five minute gap:\n%s", timer)
	}
}

// A machine that was switched off has to be scanned soon after it comes back
// rather than waiting out a whole interval, which is what OnBootSec buys.
func TestTimerFile_Always_RunsSoonAfterBoot(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalDaily})

	if !strings.Contains(timer, "OnBootSec=") {
		t.Fatalf("a machine that was off must not wait a full interval:\n%s", timer)
	}
}

// Persistent= only has an effect on OnCalendar timers. Setting it on a
// monotonic one does nothing at all while reading as though missed runs were
// covered, which is worse than not claiming it.
func TestTimerFile_MonotonicTimer_DoesNotClaimPersistence(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalDaily})

	if strings.Contains(timer, "Persistent=") && !strings.Contains(timer, "OnCalendar=") {
		t.Fatalf("Persistent= without OnCalendar= is a no-op that reads as a guarantee:\n%s", timer)
	}
}

func TestTimerFile_Always_StartsTheServiceUnit(t *testing.T) {
	timer := timerFile(Plan{Interval: config.IntervalWeekly})

	if !strings.Contains(timer, "Unit="+Name+".service") {
		t.Fatalf("expected the timer bound to the service:\n%s", timer)
	}
}

func TestJitter_ShortInterval_StaysWellUnderIt(t *testing.T) {
	if spread := jitter(300); spread >= 300 {
		t.Fatalf("a spread of %ds would double a five minute interval", spread)
	}
}

// Every machine on a weekly schedule waking at the same second would arrive as
// one burst, so the spread is capped rather than proportional forever.
func TestJitter_LongInterval_IsCapped(t *testing.T) {
	if spread := jitter(604800); spread != 300 {
		t.Fatalf("expected the spread capped at 300s, got %d", spread)
	}
}

func TestJitter_TinyInterval_IsStillPositive(t *testing.T) {
	if spread := jitter(5); spread < 1 {
		t.Fatalf("RandomizedDelaySec must be positive, got %d", spread)
	}
}

func TestQuote_PathWithASpace_IsQuoted(t *testing.T) {
	if got := quote("/home/a b/stackdrift"); got != `"/home/a b/stackdrift"` {
		t.Fatalf("systemd splits on spaces, got %s", got)
	}
}

func TestQuote_PlainPath_IsLeftAlone(t *testing.T) {
	if got := quote("/usr/local/bin/stackdrift"); got != "/usr/local/bin/stackdrift" {
		t.Fatalf("expected no quoting, got %s", got)
	}
}

func TestUnitFile_PathWithASpace_QuotesTheExec(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/opt/my tools/stackdrift"}, false)

	if !strings.Contains(unit, `ExecStart="/opt/my tools/stackdrift" watch`) {
		t.Fatalf("expected the binary path quoted:\n%s", unit)
	}
}

// One service per machine, not one per scanned directory. The command it runs
// takes no directory: the sweep reads every linked project itself. A unit bound
// to one directory would need a second unit for the next project scanned.
func TestUnitFile_Always_SweepsEveryProjectRatherThanOneDirectory(t *testing.T) {
	unit := unitFile(Plan{Interval: config.IntervalDaily, Exec: "/x/stackdrift"}, false)

	if strings.Contains(unit, "WorkingDirectory=") {
		t.Fatalf("a unit tied to one directory cannot cover the next project:\n%s", unit)
	}

	for _, line := range strings.Split(unit, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "ExecStart=")
		if !found {
			continue
		}
		if strings.TrimSpace(rest) != "/x/stackdrift watch" {
			t.Fatalf("the sweep takes no directory argument, got %q", rest)
		}
	}
}
