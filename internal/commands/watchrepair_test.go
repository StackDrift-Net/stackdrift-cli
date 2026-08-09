package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

func healthy() service.State {
	return service.State{Installed: true, Enabled: true, Running: true}
}

func TestRepairReason_SchedulerHoldsIt_IsNotRepaired(t *testing.T) {
	if reason := repairReason(healthy(), "/usr/local/bin/stackdrift", true); reason != "" {
		t.Fatalf("a healthy service must be left alone, got %q", reason)
	}
}

// qc-dev and qc-prod. The unit files were on disk, so every check that asked
// only whether the file existed said the machine was covered.
func TestRepairReason_InstalledButNeverEnabled_IsRepaired(t *testing.T) {
	reason := repairReason(service.State{Installed: true}, "/usr/local/bin/stackdrift", true)

	if !strings.Contains(reason, "enabled") {
		t.Fatalf("want the reason to name the enable that never took, got %q", reason)
	}
}

// spnix. Enabled on disk and the running manager had never loaded it.
func TestRepairReason_EnabledButNotHeld_IsRepaired(t *testing.T) {
	reason := repairReason(service.State{Installed: true, Enabled: true}, "/usr/local/bin/stackdrift", true)

	if reason == "" {
		t.Fatal("a service the scheduler is not holding runs nothing and must be repaired")
	}
}

func TestRepairReason_UnitHasGone_IsRepaired(t *testing.T) {
	reason := repairReason(service.State{}, "", false)

	if reason == "" {
		t.Fatal("a service that is meant to be installed and is not must be repaired")
	}
}

// qc-dev again. The unit pointed at a copy of the CLI under ~/.dotnet/tools
// that had been replaced by one in /usr/local/bin, so an enabled and armed
// timer would still have failed at 203/EXEC every night.
func TestRepairReason_PointsAtABinaryThatIsGone_IsRepaired(t *testing.T) {
	reason := repairReason(healthy(), "/home/ubuntu/.dotnet/tools/stackdrift", false)

	if !strings.Contains(reason, "binary") {
		t.Fatalf("want the reason to name the missing binary, got %q", reason)
	}
}

// Windows cannot read its own task reliably, because the field it would have to
// parse is localised. An empty answer there is "do not know", and repairing a
// healthy service on the strength of it would reinstall on every single scan.
func TestRepairReason_ExecCouldNotBeRead_IsNotRepaired(t *testing.T) {
	if reason := repairReason(healthy(), "", false); reason != "" {
		t.Fatalf("an unreadable exec path proves nothing, got %q", reason)
	}
}

func TestRepairWatchService_NothingWasEverInstalled_DoesNotInstallOne(t *testing.T) {
	swapRepairProbes(t, service.State{}, "")
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: false})

	if len(*plans) > 0 {
		t.Fatalf("someone who declined or uninstalled must never have one put back, got %+v", *plans)
	}
}

func TestRepairWatchService_Healthy_InstallsNothing(t *testing.T) {
	swapRepairProbes(t, healthy(), exeForRepairTest(t))
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalDaily})

	if len(*plans) > 0 {
		t.Fatalf("a working service must not be reinstalled on every scan, got %+v", *plans)
	}
}

func TestRepairWatchService_NotEnabled_ReinstallsAtTheSavedInterval(t *testing.T) {
	swapRepairProbes(t, service.State{Installed: true}, exeForRepairTest(t))
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalWeekly})

	if len(*plans) != 1 {
		t.Fatalf("expected one repair, got %d", len(*plans))
	}
	if (*plans)[0].Interval != config.IntervalWeekly {
		t.Fatalf("the repair must keep the interval they chose, got %q", (*plans)[0].Interval)
	}
}

// A machine whose saved interval is missing still has to end up on a schedule,
// because the alternative is an install that silently does nothing.
func TestRepairWatchService_NoSavedInterval_StillPicksOne(t *testing.T) {
	swapRepairProbes(t, service.State{Installed: true}, exeForRepairTest(t))
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: true})

	if len(*plans) != 1 || !config.KnownInterval((*plans)[0].Interval) {
		t.Fatalf("the repair has to install a real schedule, got %+v", *plans)
	}
}

func exeForRepairTest(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stackdrift")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("could not stage a binary: %v", err)
	}
	return path
}

func recordRepairInstalls(t *testing.T) *[]service.Plan {
	t.Helper()

	var plans []service.Plan
	original := installPlan
	installPlan = func(plan service.Plan) error {
		plans = append(plans, plan)
		return nil
	}
	t.Cleanup(func() { installPlan = original })
	return &plans
}

func swapRepairProbes(t *testing.T, state service.State, exe string) *int {
	t.Helper()

	calls := 0
	previousStatus, previousExec := statusOf, installedExecOf
	statusOf = func() (service.State, error) {
		calls++
		return state, nil
	}
	installedExecOf = func() string { return exe }
	t.Cleanup(func() { statusOf, installedExecOf = previousStatus, previousExec })
	return &calls
}

// A binary that is genuinely gone is the case the repair exists for.
func TestExists_PathThatIsNotThere_IsMissing(t *testing.T) {
	if exists(filepath.Join(t.TempDir(), "stackdrift")) {
		t.Fatal("a path with nothing at it does not exist")
	}
}

// A path this process is not allowed to look at is UNKNOWN, not gone. On
// Windows the scheduled task is machine-wide and can name a binary under
// another account's profile, which cannot be traversed. Reading that as a
// deleted binary made every scan take the task over from the other account, and
// they took it back on their next scan.
func TestExists_PathThatCannotBeRead_IsNotTreatedAsMissing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can traverse anything, so the permission error cannot be staged")
	}

	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatalf("could not stage an unreadable directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	if !exists(filepath.Join(locked, "stackdrift")) {
		t.Fatal("a path that cannot be looked at must not be read as one that has gone")
	}
}

// A repair must not repoint the scheduler at whatever copy of the CLI happens
// to be running it. Whoever is diagnosing a dead watcher is quite likely running
// a build from a downloads folder, and installing that path would leave every
// scheduled run failing the moment they deleted it.
func TestRepairWatchService_UnitPointsAtALivingBinary_KeepsThatPath(t *testing.T) {
	installed := exeForRepairTest(t)
	swapRepairProbes(t, service.State{Installed: true}, installed)
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalDaily})

	if len(*plans) != 1 {
		t.Fatalf("expected one repair, got %d", len(*plans))
	}
	if (*plans)[0].Exec != installed {
		t.Fatalf("the repair must keep the installed binary %q, got %q", installed, (*plans)[0].Exec)
	}
}

// The one case that does need a fresh path: the binary the unit names has gone.
func TestRepairWatchService_UnitPointsAtAMissingBinary_UsesTheRunningOne(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "stackdrift")
	swapRepairProbes(t, healthy(), gone)
	plans := recordRepairInstalls(t)
	t.Setenv("HOME", t.TempDir())

	repairWatchService(&config.WatchSettings{Asked: true, Enabled: true, Interval: config.IntervalDaily})

	if len(*plans) != 1 {
		t.Fatalf("expected one repair, got %d", len(*plans))
	}
	if (*plans)[0].Exec == gone {
		t.Fatal("a unit pointing at a binary that has gone must be repointed")
	}
}
