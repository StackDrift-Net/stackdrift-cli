package service

import (
	"strings"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
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

// taskStatus reads whether the task is live. Ready means scheduled and waiting;
// Running means a scan is happening right now.
//
// Only the status field is read. Searching the whole body was wrong: the
// non-verbose /fo LIST output always carries a "Logon Mode:" field, so matching
// it for the schedule reported every task as near realtime whatever it was set
// to, and words like "Ready" turn up in task names and folders as well.
func taskStatus(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "Status:")
		if !found {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(rest))
		return status == "ready" || status == "running"
	}
	return false
}
