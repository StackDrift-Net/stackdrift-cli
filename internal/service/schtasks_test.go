package service

import (
	"strings"
	"testing"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/config"
)

// Go builds the Windows command line itself and never goes through cmd.exe, so
// it already escapes each argument. Pre-escaping the quotes for a shell that is
// not there put literal backslashes into the registered command and the task
// failed to start.
func TestQuoteTaskPath_PathWithASpace_UsesPlainQuotes(t *testing.T) {
	got := quoteTaskPath(`C:\Program Files\StackDrift\stackdrift.exe`)

	if strings.Contains(got, `\"`) {
		t.Fatalf("cmd.exe escaping reaches schtasks literally, got %s", got)
	}
	if got != `"C:\Program Files\StackDrift\stackdrift.exe"` {
		t.Fatalf("expected plain quotes, got %s", got)
	}
}

func TestQuoteTaskPath_PlainPath_IsLeftAlone(t *testing.T) {
	if got := quoteTaskPath(`C:\Tools\stackdrift.exe`); got != `C:\Tools\stackdrift.exe` {
		t.Fatalf("expected no quoting, got %s", got)
	}
}

func TestTaskSchedule_EveryKnownInterval_HasATiming(t *testing.T) {
	for _, interval := range []string{
		config.IntervalFiveMin, config.IntervalHourly, config.IntervalTwiceDay,
		config.IntervalDaily, config.IntervalEveryOtherDay, config.IntervalWeekly,
	} {
		args := taskSchedule(interval)
		if len(args) < 2 || args[0] != "/sc" {
			t.Fatalf("%q produced no schedule, got %v", interval, args)
		}
	}
}

func TestTaskSchedule_TwiceDaily_IsTwelveHourly(t *testing.T) {
	got := strings.Join(taskSchedule(config.IntervalTwiceDay), " ")

	if got != "/sc HOURLY /mo 12" {
		t.Fatalf("expected every twelve hours, got %q", got)
	}
}

// The only offered interval schtasks has no word for. DAILY with a modifier of
// two is how it is said; without the modifier it would quietly run every day.
func TestTaskSchedule_EveryOtherDay_IsDailyWithATwoDayModifier(t *testing.T) {
	got := strings.Join(taskSchedule(config.IntervalEveryOtherDay), " ")

	if got != "/sc DAILY /mo 2" {
		t.Fatalf("expected every second day, got %q", got)
	}
}

func TestTaskSchedule_FiveMinutes_UsesTheMinuteModifier(t *testing.T) {
	got := strings.Join(taskSchedule(config.IntervalFiveMin), " ")

	if got != "/sc MINUTE /mo 5" {
		t.Fatalf("expected every five minutes, got %q", got)
	}
}

// Non-verbose schtasks output always carries a "Logon Mode:" field, so matching
// the whole body for the word reported every task as near realtime whatever it
// was really set to. Only the status field is read now.
func TestTaskStatus_ReadyTaskWithALogonModeField_IsRunning(t *testing.T) {
	output := "TaskName: \\StackDrift Watch\r\nNext Run Time: 30/07/2026 14:00:00\r\n" +
		"Status: Ready\r\nLogon Mode: Interactive/Background\r\n"

	if !taskStatus(output) {
		t.Fatalf("a ready task is live:\n%s", output)
	}
}

func TestTaskStatus_DisabledTask_IsNotRunning(t *testing.T) {
	output := "TaskName: \\StackDrift Watch\r\nStatus: Disabled\r\nLogon Mode: Interactive/Background\r\n"

	if taskStatus(output) {
		t.Fatalf("a disabled task is not live:\n%s", output)
	}
}

func TestTaskStatus_RunningTask_IsRunning(t *testing.T) {
	if !taskStatus("Status: Running\r\n") {
		t.Fatal("a task mid-scan is live")
	}
}

// The word "Ready" appears in the task name and folder of plenty of real
// installs. Reading the whole body would match those too.
func TestTaskStatus_WordAppearsOutsideTheStatusField_IsIgnored(t *testing.T) {
	output := "TaskName: \\Ready Player One\\StackDrift Watch\r\nStatus: Disabled\r\n"

	if taskStatus(output) {
		t.Fatalf("only the status field decides:\n%s", output)
	}
}

func TestTaskStatus_NoStatusField_IsNotRunning(t *testing.T) {
	if taskStatus("TaskName: \\StackDrift Watch\r\n") {
		t.Fatal("output with no status must not be read as running")
	}
}
