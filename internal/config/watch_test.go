package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWatch_NothingSaved_ReportsNotAsked(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	if settings := LoadWatch(); settings.Asked {
		t.Fatalf("a machine that has never been offered the service must read as unasked, got %+v", settings)
	}
}

func TestSaveAndLoadWatch_RoundTrips(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	if err := SaveWatch(&WatchSettings{Asked: true, Enabled: true, Interval: IntervalHourly}); err != nil {
		t.Fatal(err)
	}

	loaded := LoadWatch()
	if !loaded.Asked || !loaded.Enabled || loaded.Interval != IntervalHourly {
		t.Fatalf("expected the choice remembered, got %+v", loaded)
	}
}

// Declining is a decision, and it has to survive so the question is not put
// again after every single scan.
func TestSaveAndLoadWatch_Declined_StaysDeclined(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	if err := SaveWatch(&WatchSettings{Asked: true, Enabled: false}); err != nil {
		t.Fatal(err)
	}

	loaded := LoadWatch()
	if !loaded.Asked {
		t.Fatal("a decline must be remembered as having been asked")
	}
	if loaded.Enabled {
		t.Fatal("a decline must not read as enabled")
	}
}

// A preferences file that will not parse must not stop a scan. Asking once more
// is a better answer than refusing to run.
func TestLoadWatch_CorruptFile_ReadsAsUnasked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STACKDRIFT_HOME", home)

	if err := os.WriteFile(filepath.Join(home, WatchFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if settings := LoadWatch(); settings.Asked {
		t.Fatalf("a damaged file must not read as a decision, got %+v", settings)
	}
}

func TestIntervalSeconds_EveryKnownInterval_HasADuration(t *testing.T) {
	known := []string{
		IntervalRealtime, IntervalFiveMin, IntervalHourly, IntervalTwiceDay,
		IntervalDaily, IntervalEveryOtherDay, IntervalWeekly,
	}

	for _, interval := range known {
		if IntervalSeconds(interval) <= 0 {
			t.Fatalf("%q is offered to the user but has no duration", interval)
		}
		if !KnownInterval(interval) {
			t.Fatalf("%q is offered to the user but is not recognised", interval)
		}
	}
}

func TestIntervalSeconds_Unknown_IsRejected(t *testing.T) {
	if KnownInterval("fortnightly") {
		t.Fatal("an interval nobody offers must not be accepted")
	}
}

// Realtime's number is the gap between two cheap stat sweeps, not between
// scans, so it must be far shorter than the shortest scheduled interval or the
// name is a lie.
func TestIntervalSeconds_Realtime_IsShorterThanEveryScheduledInterval(t *testing.T) {
	realtime := IntervalSeconds(IntervalRealtime)
	shortest := IntervalSeconds(IntervalFiveMin)

	if realtime >= shortest {
		t.Fatalf("realtime sweeps every %ds, which is no better than the %ds option", realtime, shortest)
	}
}

// The name has to mean what it says, or a person picking it gets a schedule
// nobody described to them.
func TestIntervalSeconds_EveryOtherDay_IsTwoDays(t *testing.T) {
	if got := IntervalSeconds(IntervalEveryOtherDay); got != 2*IntervalSeconds(IntervalDaily) {
		t.Fatalf("expected twice the daily interval, got %ds", got)
	}
}

func TestIntervalLabel_EveryOtherDay_ReadsAsItIsOffered(t *testing.T) {
	if got := IntervalLabel(IntervalEveryOtherDay); got != "every other day" {
		t.Fatalf("expected a label that matches the menu, got %q", got)
	}
}

func TestLinkedProjects_SeveralLinks_ReturnsThemAll(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	for _, id := range []int{4, 9} {
		if err := SaveProject(t.TempDir(), &ProjectConfig{ProjectID: id, ProjectName: "P"}); err != nil {
			t.Fatal(err)
		}
	}

	projects, err := LinkedProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected both links, got %d", len(projects))
	}
}

func TestLinkedProjects_NothingScannedYet_ReturnsNothing(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", filepath.Join(t.TempDir(), "missing"))

	projects, err := LinkedProjects()
	if err != nil {
		t.Fatalf("an unscanned machine is not an error, got %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected no projects, got %d", len(projects))
	}
}

// The watcher writes a digest of what it uploaded so it can tell a lock file
// that moved from one it has already sent.
func TestSaveAndLoadProject_GroupDigest_RoundTrips(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	dir := t.TempDir()
	cfg := &ProjectConfig{
		ProjectID: 3,
		DependencyGrp: []TrackedDependencyGroup{
			{Name: "web npm", Ecosystem: "Npm", Digest: "abc123"},
		},
	}
	if err := SaveProject(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DependencyGrp[0].Digest != "abc123" {
		t.Fatalf("expected the digest kept, got %q", loaded.DependencyGrp[0].Digest)
	}
}

// The answer to the service offer belongs to the machine, not to the directory
// that happened to be scanned when it was put. Scanning a second project must
// not put it again, because one service already covers every linked project.
func TestLoadWatch_AnsweredWhileScanningOneProject_StaysAnsweredForTheNext(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())

	if err := SaveProject(t.TempDir(), &ProjectConfig{ProjectID: 4, ProjectName: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWatch(&WatchSettings{Asked: true, Enabled: true, Interval: IntervalDaily}); err != nil {
		t.Fatal(err)
	}

	if err := SaveProject(t.TempDir(), &ProjectConfig{ProjectID: 9, ProjectName: "Second"}); err != nil {
		t.Fatal(err)
	}

	settings := LoadWatch()
	if !settings.Asked || !settings.Enabled || settings.Interval != IntervalDaily {
		t.Fatalf("a second project must not reset the machine's answer, got %+v", settings)
	}
}
