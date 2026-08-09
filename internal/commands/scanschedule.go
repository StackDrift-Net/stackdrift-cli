package commands

import "github.com/StackDrift-Net/stackdrift-cli/internal/config"

// scheduleHours says how often this machine has undertaken to check the system,
// which is the only way the website can tell a system that stopped reporting
// from one nobody ever put on a schedule.
//
// Two answers only: a number, or nothing.
//
// There is deliberately no way to say "this system is no longer on a schedule",
// and that is the whole design decision here. A system can be scanned from
// several machines, watch.json is per MACHINE, and the report carries no
// machine identity at all, so the server cannot tell which machine a claim is
// about. Any message meaning "clear the schedule" is therefore a message one
// machine can send about another machine's watcher, and it would permanently
// disable the overdue check for a system that was being watched perfectly well.
// A laptop that once declined the offer, or once uninstalled a watcher, would
// silently switch the alarm off for every system it ever scanned.
//
// So a machine only ever reports a schedule it is actually keeping. The stored
// value moves only when a live watcher says so.
//
// The cost is that removing the last watcher for a system leaves the stored
// interval behind, and the system is eventually flagged as no longer checked.
// That is the correct answer rather than a shortcoming: nothing IS checking it.
// Clearing it properly needs the report to say which machine it came from, and
// that is a bigger change than this one.
func scheduleHours(settings *config.WatchSettings) *int {
	if settings == nil || !settings.Enabled {
		return nil
	}

	seconds := config.IntervalSeconds(settings.Interval)
	if seconds <= 0 {
		// Enabled with an interval nothing recognises. Windows reaches this
		// honestly: its Status cannot read the schedule back out of the task,
		// so a machine whose watch.json was lost adopts the task with no
		// interval at all. Reporting a guess would be worse than saying
		// nothing, because guessing a shorter interval than the real one raises
		// a "checks have stopped" alarm on a machine that is working.
		return nil
	}

	// Rounded up, and never below an hour. A resident watcher sweeps in
	// seconds, and reporting that would have the website calling a system
	// overdue within minutes of a check that worked perfectly.
	hours := (seconds + 3599) / 3600
	if hours < 1 {
		hours = 1
	}
	return &hours
}
