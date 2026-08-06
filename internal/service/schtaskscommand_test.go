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
