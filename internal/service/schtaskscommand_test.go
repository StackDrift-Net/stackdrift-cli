package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

// schtasks takes one command string and has no way to set an environment
// variable, so the marker that says "the scheduler started this" has to be an
// argument. Kept here rather than in service_windows.go so it is tested on
// every platform, the same reason quoteTaskPath and taskSchedule live here.

func TestTaskCommand_Scheduled_MarksTheRun(t *testing.T) {
	got := taskCommand(`C:\Users\u\stackdrift.exe`, config.IntervalDaily)

	if !strings.HasSuffix(got, "watch --scheduled") {
		t.Fatalf("expected the run marked as the scheduler's, got %q", got)
	}
}

func TestTaskCommand_Realtime_StaysResidentAndMarksTheRun(t *testing.T) {
	got := taskCommand(`C:\Users\u\stackdrift.exe`, config.IntervalRealtime)

	if !strings.Contains(got, "--resident") {
		t.Fatalf("near realtime has to stay resident, got %q", got)
	}
	if !strings.Contains(got, "--scheduled") {
		t.Fatalf("expected the run marked as the scheduler's, got %q", got)
	}
}

func TestTaskCommand_PathWithASpace_IsQuoted(t *testing.T) {
	got := taskCommand(`C:\Program Files\StackDrift\stackdrift.exe`, config.IntervalDaily)

	if !strings.HasPrefix(got, `"C:\Program Files\StackDrift\stackdrift.exe"`) {
		t.Fatalf("expected the path quoted, got %q", got)
	}
}

// A scheduled task takes its creation time as the trigger, so a weekly one
// created at noon does not run for seven days. Until it runs, the website is
// still holding the interval this machine reported LAST time and computing its
// overdue deadline from that, so choosing a SHORTER interval would raise a
// "checks have stopped" alarm on a machine that had just been made healthier.
// Starting it now is what closes that window. Linux gets the same property from
// OnBootSec=2min and macOS from RunAtLoad.
func TestTaskInstallSteps_EveryInterval_StartsTheTaskImmediately(t *testing.T) {
	for _, interval := range []string{
		config.IntervalDaily,
		config.IntervalEveryOtherDay,
		config.IntervalWeekly,
		config.IntervalHourly,
		config.IntervalRealtime,
	} {
		steps := taskInstallSteps(Plan{Interval: interval, Exec: `C:\stackdrift.exe`})

		last := steps[len(steps)-1]
		if len(last.Args) == 0 || last.Args[0] != "/run" {
			t.Fatalf("%q has to be started as part of installing it, steps were %+v", interval, steps)
		}
	}
}

// The teardown has to precede the create, or the create fails on an existing
// task; and /end has to precede /delete, or an instance the old definition
// started outlives it.
func TestTaskInstallSteps_TearsDownBeforeCreating(t *testing.T) {
	steps := taskInstallSteps(Plan{Interval: config.IntervalDaily, Exec: `C:\stackdrift.exe`})

	var order []string
	for _, step := range steps {
		order = append(order, step.Args[0])
	}

	want := []string{"/end", "/delete", "/create", "/run"}
	if len(order) != len(want) {
		t.Fatalf("expected %v, got %v", want, order)
	}
	for i, verb := range want {
		if order[i] != verb {
			t.Fatalf("expected %v, got %v", want, order)
		}
	}
}

func TestTaskInstallSteps_Realtime_StartsAtLogonRatherThanOnASchedule(t *testing.T) {
	steps := taskInstallSteps(Plan{Interval: config.IntervalRealtime, Exec: `C:\stackdrift.exe`})

	create := strings.Join(steps[2].Args, " ")
	if !strings.Contains(create, "/sc ONLOGON") {
		t.Fatalf("a resident watcher has no schedule of its own, got %q", create)
	}
}

func TestTaskInstallSteps_Daily_CarriesTheDailySchedule(t *testing.T) {
	steps := taskInstallSteps(Plan{Interval: config.IntervalDaily, Exec: `C:\stackdrift.exe`})

	create := strings.Join(steps[2].Args, " ")
	if !strings.Contains(create, "/sc DAILY") {
		t.Fatalf("expected a daily schedule, got %q", create)
	}
}

// Creating the task IS the install; everything else is housekeeping. Making the
// kick-off fatal deleted the user's working task, registered the new one, then
// reported failure so the preference was never saved, leaving the machine on
// one schedule while the CLI reported another.
func TestTaskInstallSteps_OnlyTheCreateIsRequired(t *testing.T) {
	for _, step := range taskInstallSteps(Plan{Interval: config.IntervalDaily, Exec: `C:\stackdrift.exe`}) {
		verb := step.Args[0]
		if verb == "/create" && !step.Required {
			t.Fatal("an install that could not create the task has failed")
		}
		if verb != "/create" && step.Required {
			t.Fatalf("%q must not be able to fail an install that already registered the task", verb)
		}
	}
}
