package commands

import (
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestScheduleHours_DailyWatcher_ReportsTwentyFour(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalDaily})

	if got == nil || *got != 24 {
		t.Fatalf("want 24 hours, got %v", got)
	}
}

func TestScheduleHours_WeeklyWatcher_ReportsAWeek(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalWeekly})

	if got == nil || *got != 24*7 {
		t.Fatalf("want a week in hours, got %v", got)
	}
}

// A resident watcher sweeps in seconds. Rounding that to zero would tell the
// website every system was overdue the moment it looked away.
func TestScheduleHours_ResidentWatcher_IsAtLeastAnHour(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalRealtime})

	if got == nil || *got < 1 {
		t.Fatalf("want at least one hour, got %v", got)
	}
}

// Nothing may ever say "clear the schedule". The report carries no machine
// identity, so a machine that removed ITS watcher cannot be distinguished from
// one speaking about a system another machine still watches faithfully. A
// message that clears would let any laptop switch the alarm off for every
// system it scans.
//
// Leaving the stored interval behind means the system is eventually flagged as
// no longer checked, which is the true answer once its last watcher is gone.
func TestScheduleHours_WatcherWasUninstalled_SaysNothing(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: false, Interval: config.IntervalDaily})

	if got != nil {
		t.Fatalf("no machine may clear a schedule it cannot prove is its own, got %v", got)
	}
}

// Declining the offer and uninstalling a watcher both leave Asked true and
// Enabled false, and neither may clear anything.
//
// Getting this wrong is worse than it sounds: watch.json is per machine, not
// per system, so one laptop where somebody said no would clear the schedule of
// every system it ever scans, including ones a server is faithfully watching.
// The next time that server died, nothing would notice, which is the exact
// failure the schedule exists to catch.
func TestScheduleHours_DeclinedTheOfferAndNeverHadOne_SaysNothing(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: false})

	if got != nil {
		t.Fatalf("a machine that never had a schedule cannot report losing one, got %v", got)
	}
}

// A machine that has never been offered a watcher says nothing at all. Systems
// are scanned from more than one machine, and a hand-run scan on a laptop must
// not wipe the schedule a server is keeping.
func TestScheduleHours_NeverOfferedAWatcher_SaysNothing(t *testing.T) {
	if got := scheduleHours(&config.WatchSettings{}); got != nil {
		t.Fatalf("want nothing reported, got %v", got)
	}
}

func TestScheduleHours_NoSettingsAtAll_SaysNothing(t *testing.T) {
	if got := scheduleHours(nil); got != nil {
		t.Fatalf("want nothing reported, got %v", got)
	}
}

// Enabled with an interval nothing recognises. Reporting a made up number would
// be worse than reporting none.
func TestScheduleHours_UnreadableInterval_SaysNothing(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: true, Interval: "fortnightly"})

	if got != nil {
		t.Fatalf("want nothing reported, got %v", got)
	}
}

// Windows Status cannot recover the interval from a registered task, so a
// machine whose watch.json was lost adopts the task with none. Reporting a
// guess would be worse than silence: guessing shorter than the real schedule
// raises a "checks have stopped" alarm on a machine that is working fine.
func TestScheduleHours_EnabledButTheIntervalIsUnknown_SaysNothing(t *testing.T) {
	got := scheduleHours(&config.WatchSettings{Asked: true, Enabled: true, Interval: ""})

	if got != nil {
		t.Fatalf("a guess is worse than saying nothing, got %v", got)
	}
}
