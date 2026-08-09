package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
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

func TestTaskEnabled_ReadyTask_IsEnabled(t *testing.T) {
	if !taskEnabled("Status: Ready\r\n") {
		t.Fatal("a scheduled and waiting task is enabled")
	}
}

// The distinction this flag exists for. A disabled task is still registered, so
// every file-existence check reports it as installed while it never runs again.
func TestTaskEnabled_DisabledTask_IsNotEnabled(t *testing.T) {
	if taskEnabled("TaskName: \\StackDrift Watch\r\nStatus: Disabled\r\n") {
		t.Fatal("a disabled task will never run, so it is not enabled")
	}
}

// A task mid-scan is obviously not switched off, and reading it as such during
// the seconds a sweep takes would fail a verified install at random.
func TestTaskEnabled_RunningTask_IsEnabled(t *testing.T) {
	if !taskEnabled("Status: Running\r\n") {
		t.Fatal("a task mid-scan is enabled")
	}
}

// schtasks answered, the task is there, and the field is missing or in a
// language this does not read. Guessing "disabled" would roll back a good
// install on every non-English Windows.
func TestTaskEnabled_NoStatusField_IsEnabled(t *testing.T) {
	if !taskEnabled("TaskName: \\StackDrift Watch\r\n") {
		t.Fatal("only an explicit Disabled means disabled")
	}
}

// The regression this replaced taskStatus for. taskStatus read the status field
// as an English word and returned false for anything else, so on a German or
// Japanese Windows a perfectly good task reported as not running. Once
// verifyInstalled started treating "not running" as a failed install, that
// rejected every install on a non-English machine and left the preference
// unsaved, so the CLI reinstalled a working task on every scan.
func TestTaskEnabled_LocalisedStatus_IsStillLive(t *testing.T) {
	for _, output := range []string{
		"TaskName: \\StackDrift Watch\r\nStatus: Bereit\r\n",
		"TaskName: \\StackDrift Watch\r\nStatut: Prêt\r\n",
		"TaskName: \\StackDrift Watch\r\nStatus: 準備完了\r\n",
	} {
		if !taskEnabled(output) {
			t.Fatalf("a registered task must not be read as dead because its status is not in English:\n%s", output)
		}
	}
}

// The one word that does mean the scheduler will not start it, whatever the
// casing schtasks happens to use.
func TestTaskEnabled_DisabledInAnyCasing_IsNotLive(t *testing.T) {
	for _, output := range []string{"Status: Disabled\r\n", "Status: DISABLED\r\n", "Status: disabled\r\n"} {
		if taskEnabled(output) {
			t.Fatalf("a disabled task will never run:\n%s", output)
		}
	}
}

// "Ready" turns up in task names and folders. Only the status field decides.
func TestTaskEnabled_WordAppearsOutsideTheStatusField_IsIgnored(t *testing.T) {
	if taskEnabled("TaskName: \\Ready Player One\\StackDrift Watch\r\nStatus: Disabled\r\n") {
		t.Fatalf("only the status field decides")
	}
}
