package service

import (
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// The parts of the Windows installer that are pure string work live here rather
// than in service_windows.go, with no build constraint, so their tests run on
// every platform. Only the code that actually shells out to schtasks is
// Windows-only, and that is the part no test here could execute anyway.

const taskName = "StackDrift Watch"

// quoteTaskPath wraps a path for the /tr command string.
//
// Plain quotes, not backslash-escaped ones. Go builds the Windows command line
// itself and never goes through cmd.exe, so it already escapes each argument
// and the quotes below reach schtasks as written. Pre-escaping them for a shell
// that is not there put literal backslashes into the registered command and the
// task failed to start.
func quoteTaskPath(value string) string {
	if !strings.ContainsAny(value, " \t") {
		return value
	}
	return `"` + value + `"`
}

// taskCommand builds the whole /tr command string.
//
// schtasks takes one command line and has no way to set an environment
// variable for the task, so the marker that says the scheduler started this run
// has to be an argument. That is why ScheduledFlag is an argument on the other
// two platforms as well rather than each doing something different.
func taskCommand(exe, interval string) string {
	command := quoteTaskPath(exe) + " watch"
	if interval == config.IntervalRealtime {
		// A resident watcher has no schedule of its own, it starts at logon and
		// stays up doing its own waiting
		command += " --resident"
	}
	return command + " " + ScheduledFlag
}

// execFromTask reads the binary out of the "Task To Run" field, which only the
// verbose query prints. The command is the path followed by its arguments, and
// a path holding a space is quoted, so the quotes are what bound it.
func execFromTask(output string) string {
	for _, line := range strings.Split(output, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "Task To Run:")
		if !found {
			continue
		}
		command := strings.TrimSpace(rest)
		if strings.HasPrefix(command, `"`) {
			if end := strings.Index(command[1:], `"`); end >= 0 {
				return command[1 : end+1]
			}
			return strings.Trim(command, `"`)
		}
		if space := strings.IndexAny(command, " \t"); space >= 0 {
			return command[:space]
		}
		return command
	}
	return ""
}

// taskStep is one schtasks invocation. Required says whether failing it means
// the install failed, which is a policy decision rather than a detail, so it
// lives in the data instead of in the loop that runs it.
type taskStep struct {
	Args     []string
	Required bool
}

// taskInstallSteps is every schtasks invocation an install makes, in order.
//
// It lives here rather than inline in service_windows.go for the reason the
// whole file exists: the ordering and the required/optional split carry the
// behaviour, and this way they are tested on every platform instead of only on
// the one machine nobody runs the suite on.
//
// Two of these are worth explaining.
//
// The task is STARTED immediately whatever the interval, not just for realtime.
// schtasks with no /st takes the creation time as the trigger, so a weekly task
// created at noon does not run for seven days, and until it runs the website is
// still holding the interval this machine reported LAST time and working its
// overdue deadline out from that. Shortening the wait between checks would
// therefore raise a "checks have stopped" alarm on a machine that had just been
// made healthier. Linux gets the same property free from OnBootSec=2min and
// macOS from RunAtLoad.
//
// But that start is NOT required. Once /create returns, the task is registered,
// enabled and scheduled, so the install has succeeded. A task created without
// /ru carries an InteractiveToken principal and an on-demand start can be
// refused over a non-interactive ssh, WinRM or CI session. Failing the install
// on that would leave the old task deleted, the new one running, and the
// preference unsaved, so the machine would keep one schedule while the CLI
// reported another. A missed kick-off costs only a delay.
func taskInstallSteps(plan Plan) []taskStep {
	timing := taskSchedule(plan.Interval)
	if plan.Interval == config.IntervalRealtime {
		// No schedule of its own: it starts at logon and does its own waiting.
		timing = []string{"/sc", "ONLOGON"}
	}

	create := append([]string{"/create", "/tn", taskName, "/f", "/tr", taskCommand(plan.Exec, plan.Interval)}, timing...)

	return []taskStep{
		// /end before /delete, because /delete removes the definition but
		// leaves any instance it started running. Both are expected to fail on
		// a machine that has no task yet.
		{Args: []string{"/end", "/tn", taskName}},
		{Args: []string{"/delete", "/tn", taskName, "/f"}},
		{Args: create, Required: true},
		{Args: []string{"/run", "/tn", taskName}},
	}
}

func taskSchedule(interval string) []string {
	switch interval {
	case config.IntervalFiveMin:
		return []string{"/sc", "MINUTE", "/mo", "5"}
	case config.IntervalHourly:
		return []string{"/sc", "HOURLY"}
	case config.IntervalTwiceDay:
		return []string{"/sc", "HOURLY", "/mo", "12"}
	case config.IntervalDaily:
		return []string{"/sc", "DAILY"}
	// schtasks has no word for every second day, so it is DAILY with a
	// modifier. Without /mo it would silently register a daily task.
	case config.IntervalEveryOtherDay:
		return []string{"/sc", "DAILY", "/mo", "2"}
	case config.IntervalWeekly:
		return []string{"/sc", "WEEKLY"}
	default:
		return []string{"/sc", "DAILY"}
	}
}

// taskEnabled reads whether the scheduler will ever start the task again.
//
// Only an explicit Disabled counts. Everything else, including output with no
// status field at all, is read as live: the query answered, so the task exists.
//
// There is one function here rather than two because /fo LIST localises BOTH the
// field name and its value, so nothing in this output distinguishes armed from
// enabled anywhere but English. There used to be a second function matching the
// English words "Ready" and "Running", and it failed closed on anything it could
// not parse. That was survivable while it only coloured a status line. It
// stopped being survivable when a failed reading began rejecting the install
// itself: on a German or Japanese Windows every install was rolled back over a
// task that was registered and would have run perfectly.
//
// Be honest about what this costs. Because the field NAME is localised too, the
// prefix never matches on those machines and this returns true for every
// registered task, a disabled one included. So on non-English Windows the
// enabled probe degrades to a constant and a task the user disabled by hand is
// not detected. The fail-open direction is still the right trade, because the
// alternative rejected working installs, but the durable fix is to read
// /query /xml, whose <Enabled> element is not translated. That is a bigger
// change than this one and has not been made.
//
// Only the status field is read. Searching the whole body was wrong: the
// non-verbose /fo LIST output always carries a "Logon Mode:" field, and words
// like "Ready" turn up in task names and folders as well.
func taskEnabled(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "Status:")
		if !found {
			continue
		}
		return strings.ToLower(strings.TrimSpace(rest)) != "disabled"
	}
	return true
}
