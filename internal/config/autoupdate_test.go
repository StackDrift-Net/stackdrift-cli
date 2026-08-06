package config

import (
	"os"
	"testing"
)

// Absent has to be told apart from off. Every install already in the field
// predates the question, and a machine whose owner never agreed must not start
// replacing its own binary.
func TestAutoUpdateEnabled_NeverAnswered_IsOff(t *testing.T) {
	if (&WatchSettings{Asked: true, Enabled: true}).AutoUpdateEnabled() {
		t.Fatal("a setting that was never answered must not read as on")
	}
}

func TestAutoUpdateEnabled_AnsweredNo_IsOff(t *testing.T) {
	no := false
	if (&WatchSettings{AutoUpdate: &no}).AutoUpdateEnabled() {
		t.Fatal("expected off")
	}
}

func TestAutoUpdateEnabled_AnsweredYes_IsOn(t *testing.T) {
	yes := true
	if !(&WatchSettings{AutoUpdate: &yes}).AutoUpdateEnabled() {
		t.Fatal("expected on")
	}
}

func TestAutoUpdateAnswered_TellsAbsentFromOff(t *testing.T) {
	no := false
	if (&WatchSettings{}).AutoUpdateAnswered() {
		t.Fatal("nothing saved must read as unanswered")
	}
	if !(&WatchSettings{AutoUpdate: &no}).AutoUpdateAnswered() {
		t.Fatal("an explicit no is an answer")
	}
}

func TestSaveAndLoadWatch_AutoUpdateAnswer_RoundTrips(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	yes := true
	if err := SaveWatch(&WatchSettings{Asked: true, Enabled: true, AutoUpdate: &yes}); err != nil {
		t.Fatal(err)
	}

	if loaded := LoadWatch(); !loaded.AutoUpdateEnabled() {
		t.Fatalf("expected the answer remembered, got %+v", loaded)
	}
}

// Every writer used to pass a fresh struct literal, so recording an update
// check would have wiped the interval and recording an uninstall would have
// wiped the update state. UpdateWatch exists so a writer touches only its own
// field.
func TestUpdateWatch_ChangingOneField_LeavesTheOthersAlone(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	yes := true
	if err := SaveWatch(&WatchSettings{
		Asked: true, Enabled: true, Interval: IntervalWeekly, AutoUpdate: &yes, UpdatedTo: "0.1.44",
	}); err != nil {
		t.Fatal(err)
	}

	if err := UpdateWatch(func(s *WatchSettings) { s.LastUpdateAt = "2026-08-07T00:00:00Z" }); err != nil {
		t.Fatal(err)
	}

	loaded := LoadWatch()
	if loaded.Interval != IntervalWeekly {
		t.Fatalf("the interval must survive an unrelated write, got %q", loaded.Interval)
	}
	if !loaded.AutoUpdateEnabled() {
		t.Fatal("the auto-update answer must survive an unrelated write")
	}
	if loaded.UpdatedTo != "0.1.44" {
		t.Fatalf("the recorded version must survive an unrelated write, got %q", loaded.UpdatedTo)
	}
	if loaded.LastUpdateAt != "2026-08-07T00:00:00Z" {
		t.Fatalf("expected the new stamp, got %q", loaded.LastUpdateAt)
	}
}

func TestUpdateWatch_NothingSavedYet_StillWrites(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	if err := UpdateWatch(func(s *WatchSettings) { s.UpdatedTo = "0.1.45" }); err != nil {
		t.Fatal(err)
	}

	if loaded := LoadWatch(); loaded.UpdatedTo != "0.1.45" {
		t.Fatalf("expected the write to land on a machine with no file, got %+v", loaded)
	}
}

// The file carries the answer to a question about replacing an executable, so
// it must not be readable by other accounts on a shared machine.
func TestSaveWatch_Always_WritesPrivately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STACKDRIFT_HOME", home)

	if err := SaveWatch(&WatchSettings{Asked: true}); err != nil {
		t.Fatal(err)
	}

	path, err := WatchFilePath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("expected 0600, got %o", mode)
	}
}
