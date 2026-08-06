package service

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func TestLaunchdArgs_Scheduled_MarksTheRun(t *testing.T) {
	args := launchdArgs(Plan{Interval: config.IntervalDaily, Exec: "/usr/local/bin/stackdrift"})

	want := []string{"/usr/local/bin/stackdrift", "watch", "--scheduled"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("got %v, want %v", args, want)
		}
	}
}

func TestLaunchdArgs_Realtime_StaysResident(t *testing.T) {
	args := launchdArgs(Plan{Interval: config.IntervalRealtime, Exec: "/usr/local/bin/stackdrift"})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--resident") || !strings.Contains(joined, "--scheduled") {
		t.Fatalf("got %v", args)
	}
}

func TestPlistBody_Realtime_KeepsTheAgentAlive(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalRealtime, Exec: "/usr/local/bin/stackdrift"})

	if !strings.Contains(body, "KeepAlive") {
		t.Fatalf("a resident agent that exits after updating needs launchd to start it again:\n%s", body)
	}
}

func TestPlistBody_Scheduled_CarriesTheInterval(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalWeekly, Exec: "/usr/local/bin/stackdrift"})

	if !strings.Contains(body, "StartInterval") {
		t.Fatalf("expected a scheduled agent:\n%s", body)
	}
	if intervalFromPlist(body) != config.IntervalWeekly {
		t.Fatalf("the interval has to be readable back out for status, got %q", intervalFromPlist(body))
	}
}

// launchd runs the agent unsandboxed, so there is nothing to widen and nothing
// for the auto-update answer to change here. Pinned so a later change to the
// plist has to think about it rather than silently breaking the Linux parity.
func TestPlistBody_AutoUpdateAnswer_ChangesNothing(t *testing.T) {
	on := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/usr/local/bin/stackdrift", AutoUpdate: true})
	off := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/usr/local/bin/stackdrift"})

	if on != off {
		t.Fatalf("nothing in the plist depends on the answer:\n%s\n---\n%s", on, off)
	}
}

func TestPlistBody_PathWithAnAmpersand_IsEscaped(t *testing.T) {
	body := plistBody(Plan{Interval: config.IntervalDaily, Exec: "/opt/a&b/stackdrift"})

	if strings.Contains(body, "/opt/a&b/") {
		t.Fatalf("a raw ampersand makes the plist unparseable:\n%s", body)
	}
	if !strings.Contains(body, "a&amp;b") {
		t.Fatalf("expected the path escaped:\n%s", body)
	}
}
