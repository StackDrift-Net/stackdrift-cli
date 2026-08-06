package commands

import (
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

// installPlan is swapped out because a real install writes a unit and starts a
// scheduler. What is under test is what applyServicePlan decides, which is the
// whole point of the answer being a pointer.
func capturePlan(t *testing.T) *service.Plan {
	t.Helper()

	var seen service.Plan
	original := installPlan
	installPlan = func(plan service.Plan) error {
		seen = plan
		return nil
	}
	t.Cleanup(func() { installPlan = original })
	return &seen
}

// A scripted reinstall gives no answer, and no answer is not a no. Turning off
// something the owner deliberately switched on is the bug the pointer exists
// to stop.
func TestApplyServicePlan_NoAnswerOnAMachineThatOptedIn_StaysOn(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	plan := capturePlan(t)

	yes := true
	if err := config.SaveWatch(&config.WatchSettings{Asked: true, Enabled: true, AutoUpdate: &yes}); err != nil {
		t.Fatal(err)
	}

	if err := applyServicePlan("/x/stackdrift", config.IntervalDaily, nil); err != nil {
		t.Fatal(err)
	}

	if !plan.AutoUpdate {
		t.Fatal("the scheduler entry must keep the access the update needs")
	}
	if !config.LoadWatch().AutoUpdateEnabled() {
		t.Fatal("the recorded answer must survive an install that asked nothing")
	}
}

func TestApplyServicePlan_NoAnswerOnAFreshMachine_LeavesItUnanswered(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	plan := capturePlan(t)

	if err := applyServicePlan("/x/stackdrift", config.IntervalDaily, nil); err != nil {
		t.Fatal(err)
	}

	if plan.AutoUpdate {
		t.Fatal("nothing was answered, so nothing may be widened")
	}
	if config.LoadWatch().AutoUpdateAnswered() {
		t.Fatal("an install that asked nothing must not record a decision")
	}
}

func TestApplyServicePlan_ExplicitNoOnAMachineThatOptedIn_TurnsItOff(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	plan := capturePlan(t)

	yes, no := true, false
	if err := config.SaveWatch(&config.WatchSettings{Asked: true, Enabled: true, AutoUpdate: &yes}); err != nil {
		t.Fatal(err)
	}

	if err := applyServicePlan("/x/stackdrift", config.IntervalDaily, &no); err != nil {
		t.Fatal(err)
	}

	if plan.AutoUpdate {
		t.Fatal("a declined machine keeps the tighter sandbox")
	}
	if config.LoadWatch().AutoUpdateEnabled() {
		t.Fatal("expected the decline recorded")
	}
}

// The unit and the preferences file must never disagree, or the sandbox says
// one thing and the check believes another.
func TestApplyServicePlan_ExplicitYes_MatchesWhatItRecords(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	plan := capturePlan(t)

	yes := true
	if err := applyServicePlan("/x/stackdrift", config.IntervalDaily, &yes); err != nil {
		t.Fatal(err)
	}

	if !plan.AutoUpdate || !config.LoadWatch().AutoUpdateEnabled() {
		t.Fatalf("plan says %v, settings say %v", plan.AutoUpdate, config.LoadWatch().AutoUpdateEnabled())
	}
}

// Recorded so a setting can be changed later without having to work out which
// binary the scheduler was pointed at, which cannot be read back reliably on
// every platform.
func TestApplyServicePlan_Always_RecordsTheBinaryTheSchedulerRuns(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	capturePlan(t)

	if err := applyServicePlan("/opt/sd/stackdrift", config.IntervalDaily, nil); err != nil {
		t.Fatal(err)
	}

	if got := config.LoadWatch().ServiceExec; got != "/opt/sd/stackdrift" {
		t.Fatalf("got %q", got)
	}
}

// A fresh install re-opens the question of whether the binary can be written,
// so a record from the old one must not silently keep updates off.
func TestApplyServicePlan_Always_ClearsAStaleBlockedRecord(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	capturePlan(t)

	if err := config.SaveWatch(&config.WatchSettings{UpdateBlocked: "read-only file system"}); err != nil {
		t.Fatal(err)
	}

	yes := true
	if err := applyServicePlan("/x/stackdrift", config.IntervalDaily, &yes); err != nil {
		t.Fatal(err)
	}

	if got := config.LoadWatch().UpdateBlocked; got != "" {
		t.Fatalf("expected the record cleared, got %q", got)
	}
}
