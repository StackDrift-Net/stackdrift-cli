package commands

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

func TestAutoUpdateArg_On_IsRead(t *testing.T) {
	got, ok := autoUpdateArg([]string{"install", "--auto-update"})
	if !ok || !got {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestAutoUpdateArg_Off_IsRead(t *testing.T) {
	got, ok := autoUpdateArg([]string{"install", "--no-auto-update"})
	if !ok || got {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

// Nothing passed has to stay distinguishable from an explicit no, or a scripted
// install would silently answer the question for the user.
func TestAutoUpdateArg_Absent_IsUnanswered(t *testing.T) {
	if _, ok := autoUpdateArg([]string{"install", "--interval", "daily"}); ok {
		t.Fatal("expected no answer")
	}
}

func TestAutoUpdateState_On_IsRead(t *testing.T) {
	got, ok := autoUpdateState([]string{"auto-update", "on"})
	if !ok || !got {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestAutoUpdateState_Off_IsRead(t *testing.T) {
	got, ok := autoUpdateState([]string{"auto-update", "off"})
	if !ok || got {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestAutoUpdateState_EveryAcceptedSpelling(t *testing.T) {
	for _, word := range []string{"on", "yes", "true", "enable"} {
		if got, ok := autoUpdateState([]string{"auto-update", word}); !ok || !got {
			t.Fatalf("%q should turn it on, got %v ok=%v", word, got, ok)
		}
	}
	for _, word := range []string{"off", "no", "false", "disable"} {
		if got, ok := autoUpdateState([]string{"auto-update", word}); !ok || got {
			t.Fatalf("%q should turn it off, got %v ok=%v", word, got, ok)
		}
	}
}

// Guessing here would be flipping a setting that replaces an executable.
func TestAutoUpdateState_Nonsense_IsRefused(t *testing.T) {
	if _, ok := autoUpdateState([]string{"auto-update", "maybe"}); ok {
		t.Fatal("expected an unreadable word refused rather than guessed")
	}
}

func TestAutoUpdateState_NothingGiven_IsRefused(t *testing.T) {
	if _, ok := autoUpdateState([]string{"auto-update"}); ok {
		t.Fatal("expected a bare command refused")
	}
}

// status is the surface someone reads to decide whether something is wrong, so
// a setting that replaces the binary has to be on it.
func TestStatusLines_AutoUpdateOn_SaysSo(t *testing.T) {
	on := true
	lines := statusLines(
		service.State{Installed: true, Running: true},
		config.IntervalDaily,
		&config.WatchSettings{AutoUpdate: &on, UpdatedTo: "0.1.45"},
	)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "auto-update") {
		t.Fatalf("expected auto-update reported:\n%s", joined)
	}
	if !strings.Contains(joined, "0.1.45") {
		t.Fatalf("expected the version it last installed:\n%s", joined)
	}
}

func TestStatusLines_AutoUpdateOff_SaysSo(t *testing.T) {
	off := false
	lines := statusLines(
		service.State{Installed: true, Running: true},
		config.IntervalDaily,
		&config.WatchSettings{AutoUpdate: &off},
	)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "auto-update: off") {
		t.Fatalf("expected auto-update reported as off:\n%s", joined)
	}
}

// A machine whose binary cannot be replaced would otherwise look identical to
// one that is up to date.
func TestStatusLines_UpdatesBlocked_NamesTheReason(t *testing.T) {
	on := true
	lines := statusLines(
		service.State{Installed: true, Running: true},
		config.IntervalDaily,
		&config.WatchSettings{AutoUpdate: &on, UpdateBlocked: "read-only file system"},
	)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read-only file system") {
		t.Fatalf("expected the reason shown:\n%s", joined)
	}
}

func TestStatusLines_NoServiceInstalled_SaysNothingAboutAutoUpdate(t *testing.T) {
	on := true
	lines := statusLines(service.State{}, "", &config.WatchSettings{AutoUpdate: &on})

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "auto-update") {
		t.Fatalf("there is nothing to auto-update when nothing is installed:\n%s", joined)
	}
}

// Nothing answered means nobody was asked, which has to stay apart from a no,
// or a scripted reinstall would turn off something the owner switched on.
func TestEffectiveAutoUpdate_NoAnswer_KeepsWhatTheMachineHad(t *testing.T) {
	yes := true
	if !effectiveAutoUpdate(nil, &config.WatchSettings{AutoUpdate: &yes}) {
		t.Fatal("an unanswered install must not turn off an existing opt-in")
	}
}

func TestEffectiveAutoUpdate_NoAnswerAndNeverAsked_IsOff(t *testing.T) {
	if effectiveAutoUpdate(nil, &config.WatchSettings{Asked: true, Enabled: true}) {
		t.Fatal("a machine that was never asked stays off")
	}
}

func TestEffectiveAutoUpdate_ExplicitNo_TurnsItOff(t *testing.T) {
	yes, no := true, false
	if effectiveAutoUpdate(&no, &config.WatchSettings{AutoUpdate: &yes}) {
		t.Fatal("an explicit no has to win over what was there")
	}
}

func TestEffectiveAutoUpdate_ExplicitYes_TurnsItOn(t *testing.T) {
	yes := true
	if !effectiveAutoUpdate(&yes, &config.WatchSettings{}) {
		t.Fatal("an explicit yes has to win")
	}
}

func TestResolveAutoUpdate_FlagGiven_IsHonoured(t *testing.T) {
	on := resolveAutoUpdate([]string{"install", "--auto-update"})
	if on == nil || !*on {
		t.Fatalf("got %v", on)
	}

	off := resolveAutoUpdate([]string{"install", "--no-auto-update"})
	if off == nil || *off {
		t.Fatalf("got %v", off)
	}
}

// go test hands the process a stdin that is not a terminal, which is the same
// shape a scripted install has. Nothing may be answered on the user's behalf.
func TestResolveAutoUpdate_NoFlagAndNobodyToAsk_IsUnanswered(t *testing.T) {
	if got := resolveAutoUpdate([]string{"install", "--interval", "daily"}); got != nil {
		t.Fatalf("expected no answer, got %v", *got)
	}
}

// A resident watcher can only take a new binary if something restarts it, so
// where nothing will, nothing may be promised.
func TestAppliesToInterval_ScheduledInterval_IsAlwaysApplied(t *testing.T) {
	if !appliesToInterval(config.IntervalDaily) {
		t.Fatal("a scheduled run replaces the binary and re-runs on every platform")
	}
}

func TestAppliesToInterval_Resident_FollowsWhetherTheSchedulerRestartsIt(t *testing.T) {
	if got := appliesToInterval(config.IntervalRealtime); got != service.RestartsOnExit() {
		t.Fatalf("got %v, want %v", got, service.RestartsOnExit())
	}
}

func TestStatusLines_ResidentWhereNothingRestartsIt_SaysItIsNotApplied(t *testing.T) {
	if service.RestartsOnExit() {
		t.Skip("this platform restarts a resident watcher, so the caveat does not apply")
	}

	on := true
	lines := statusLines(
		service.State{Installed: true, Running: true},
		config.IntervalRealtime,
		&config.WatchSettings{AutoUpdate: &on},
	)

	if !strings.Contains(strings.Join(lines, "\n"), "not applied") {
		t.Fatalf("expected the caveat:\n%s", strings.Join(lines, "\n"))
	}
}

// The version last installed has to survive the resident caveat, or a machine
// that switched interval loses the only record of what it is running.
func TestStatusLines_ResidentAndPreviouslyUpdated_KeepsTheVersion(t *testing.T) {
	on := true
	lines := statusLines(
		service.State{Installed: true, Running: true},
		config.IntervalRealtime,
		&config.WatchSettings{AutoUpdate: &on, UpdatedTo: "0.1.46"},
	)

	if !strings.Contains(strings.Join(lines, "\n"), "0.1.46") {
		t.Fatalf("expected the version kept:\n%s", strings.Join(lines, "\n"))
	}
}
