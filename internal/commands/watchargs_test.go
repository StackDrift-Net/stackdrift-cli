package commands

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

func TestScheduledRun_TypedByHand_IsFalse(t *testing.T) {
	if scheduledRun(nil) {
		t.Fatal("a bare watch is somebody at a keyboard, not the scheduler")
	}
}

func TestScheduledRun_ResidentByHand_IsFalse(t *testing.T) {
	if scheduledRun([]string{"--resident"}) {
		t.Fatal("running the watcher by hand is still not the scheduler")
	}
}

func TestScheduledRun_StartedByTheScheduler_IsTrue(t *testing.T) {
	if !scheduledRun([]string{service.ScheduledFlag}) {
		t.Fatal("expected the scheduler's own run recognised")
	}
}

func TestScheduledRun_ResidentScheduler_IsTrue(t *testing.T) {
	if !scheduledRun([]string{"--resident", service.ScheduledFlag}) {
		t.Fatal("expected the resident unit recognised")
	}
}

// The child has to do what this process was asked to do, or a scheduled sweep
// that updated itself would go on to run something else entirely.
func TestRerunArgs_ScheduledWatch_RepeatsTheWholeCommand(t *testing.T) {
	got := strings.Join(rerunArgs([]string{service.ScheduledFlag}), " ")

	if got != "watch --scheduled" {
		t.Fatalf("got %q", got)
	}
}

func TestRerunArgs_ResidentWatch_RepeatsTheWholeCommand(t *testing.T) {
	got := strings.Join(rerunArgs([]string{"--resident", service.ScheduledFlag}), " ")

	if got != "watch --resident --scheduled" {
		t.Fatalf("got %q", got)
	}
}

func TestRerunArgs_NoArguments_StillNamesTheCommand(t *testing.T) {
	got := strings.Join(rerunArgs(nil), " ")

	if got != "watch" {
		t.Fatalf("got %q", got)
	}
}
