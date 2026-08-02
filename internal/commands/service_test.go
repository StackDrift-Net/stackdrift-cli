package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

func TestNormalizeInterval_EveryOfferedSpelling_IsAccepted(t *testing.T) {
	cases := map[string]string{
		"realtime":      config.IntervalRealtime,
		"near-realtime": config.IntervalRealtime,
		"5m":            config.IntervalFiveMin,
		"5min":          config.IntervalFiveMin,
		"hourly":        config.IntervalHourly,
		"1h":            config.IntervalHourly,
		"twicedaily":    config.IntervalTwiceDay,
		"twice-daily":   config.IntervalTwiceDay,
		"daily":         config.IntervalDaily,
		"weekly":        config.IntervalWeekly,
	}

	for input, want := range cases {
		if got := normalizeInterval(input); got != want {
			t.Fatalf("%q should mean %q, got %q", input, want, got)
		}
	}
}

func TestNormalizeInterval_MixedCaseAndSpaces_IsAccepted(t *testing.T) {
	if got := normalizeInterval("  Hourly "); got != config.IntervalHourly {
		t.Fatalf("expected hourly, got %q", got)
	}
}

// An unrecognised interval must fall through to the question rather than
// silently installing something the user did not ask for.
func TestNormalizeInterval_Unknown_IsEmpty(t *testing.T) {
	if got := normalizeInterval("fortnightly"); got != "" {
		t.Fatalf("expected nothing for an unknown interval, got %q", got)
	}
}

func TestIntervalArg_Separate_IsRead(t *testing.T) {
	if got := intervalArg([]string{"install", "--interval", "daily"}); got != config.IntervalDaily {
		t.Fatalf("expected daily, got %q", got)
	}
}

func TestIntervalArg_Joined_IsRead(t *testing.T) {
	if got := intervalArg([]string{"install", "--interval=weekly"}); got != config.IntervalWeekly {
		t.Fatalf("expected weekly, got %q", got)
	}
}

func TestIntervalArg_FlagWithNoValue_IsEmpty(t *testing.T) {
	if got := intervalArg([]string{"install", "--interval"}); got != "" {
		t.Fatalf("a dangling flag must not be read as an interval, got %q", got)
	}
}

func TestIntervalArg_NoFlag_IsEmpty(t *testing.T) {
	if got := intervalArg([]string{"install"}); got != "" {
		t.Fatalf("expected nothing, got %q", got)
	}
}

// Every interval the picker offers has to be one the platform installers can
// actually schedule, or choosing it installs nothing.
func TestIntervalChoices_EveryOption_IsAKnownInterval(t *testing.T) {
	for _, interval := range intervalChoices {
		if !config.KnownInterval(interval) {
			t.Fatalf("%q is offered but has no duration", interval)
		}
	}
}

// Two options reading the same in the menu would be unpickable, which is the
// failure a per-interval label check cannot see. "hourly" is its own label, so
// comparing a label against its constant proves nothing.
func TestIntervalChoices_EveryOption_ReadsDifferentlyFromTheRest(t *testing.T) {
	titles := map[string]string{}
	labels := map[string]string{}

	for _, interval := range intervalChoices {
		title, label := intervalTitle(interval), config.IntervalLabel(interval)
		if title == "" || label == "" {
			t.Fatalf("%q is offered with nothing to display", interval)
		}
		if clash, taken := titles[title]; taken {
			t.Fatalf("%q and %q both read as %q in the menu", clash, interval, title)
		}
		if clash, taken := labels[label]; taken {
			t.Fatalf("%q and %q both report as %q", clash, interval, label)
		}
		titles[title], labels[label] = interval, interval
	}
}

// The default branch hands back whatever it was given, so an interval that was
// added to the menu but never given a title would show its raw constant.
func TestIntervalTitle_UnknownInterval_FallsThroughUnchanged(t *testing.T) {
	if got := intervalTitle("fortnightly"); got != "fortnightly" {
		t.Fatalf("expected the raw value back, got %q", got)
	}
}

func TestIntervalChoices_EveryOption_IsAcceptedByTheFlag(t *testing.T) {
	for _, interval := range intervalChoices {
		if got := normalizeInterval(interval); got != interval {
			t.Fatalf("%q can be picked from the menu but not passed as a flag, got %q", interval, got)
		}
	}
}

// Three options, in the order they are offered. Pinned exactly, because the
// menu is the whole interface to this feature and a fourth one appearing is a
// decision somebody should have to make deliberately.
func TestIntervalChoices_AreTheThreeOffered(t *testing.T) {
	want := []string{config.IntervalDaily, config.IntervalEveryOtherDay, config.IntervalWeekly}

	if len(intervalChoices) != len(want) {
		t.Fatalf("expected %v, got %v", want, intervalChoices)
	}
	for i, interval := range want {
		if intervalChoices[i] != interval {
			t.Fatalf("expected %v, got %v", want, intervalChoices)
		}
	}
}

// Nothing offered stays in memory between checks, so nothing offered has a
// standing cost to declare. The resident mode is still reachable with an
// explicit --interval realtime, and it is the one thing that would break this.
func TestIntervalChoices_NoneOfThemStayResident(t *testing.T) {
	for _, interval := range intervalChoices {
		if interval == config.IntervalRealtime {
			t.Fatal("the resident mode is offered again, which changes what the cost note has to say")
		}
		if strings.Contains(intervalNote(interval), residentMemory) {
			t.Fatalf("%q runs nothing between checks, so it has no standing cost to declare", interval)
		}
	}
}

// Someone with no reason to prefer one interval over another should be steered
// to the one that suits the pace advisories are actually published at.
func TestIntervalChoices_Daily_IsTheOnlyOneRecommended(t *testing.T) {
	for _, interval := range intervalChoices {
		recommended := strings.Contains(intervalNote(interval), "(recommended)")
		if interval == config.IntervalDaily && !recommended {
			t.Fatalf("daily is the recommendation, got %q", intervalNote(interval))
		}
		if interval != config.IntervalDaily && recommended {
			t.Fatalf("two recommendations is no recommendation, %q also carries one", interval)
		}
	}
}

func TestService_UnknownAction_IsRefused(t *testing.T) {
	err := Service([]string{"reticulate"})

	if err == nil {
		t.Fatal("expected an unknown action to be refused")
	}
	if !strings.Contains(err.Error(), "reticulate") {
		t.Fatalf("the error should name what was not understood, got %v", err)
	}
}

func statusLine(t *testing.T, lines []string, prefix string) string {
	t.Helper()

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no %q line in %+v", prefix, lines)
	return ""
}

// A timer armed for a daily sweep is idle almost all the time, so "running"
// described the healthy state as if something were wrong.
func TestStatusLines_Armed_ReadsAsScheduled(t *testing.T) {
	lines := statusLines(service.State{Installed: true, Running: true}, config.IntervalDaily)

	if got := statusLine(t, lines, "state:"); got != "state:    installed and scheduled" {
		t.Fatalf("unexpected state line %q", got)
	}
}

// The state Vince hit on spnix: enabled on disk, never pulled into the running
// systemd manager, so nothing was scheduled at all. This must stay alarming.
func TestStatusLines_NotArmed_StillSaysItIsNotRunning(t *testing.T) {
	lines := statusLines(service.State{Installed: true, Running: false}, config.IntervalDaily)

	if got := statusLine(t, lines, "state:"); got != "state:    installed but not running" {
		t.Fatalf("a service the scheduler is not holding must not read as scheduled, got %q", got)
	}
}

func TestStatusLines_NotInstalled_OffersTheInstallCommand(t *testing.T) {
	lines := statusLines(service.State{}, "")

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "stackdrift service install") {
		t.Fatalf("expected the install hint, got %q", joined)
	}
	if strings.Contains(joined, "state:") {
		t.Fatalf("a service that does not exist has no state to report, got %q", joined)
	}
}

func TestStatusLines_IntervalMissingFromTheState_UsesTheFallback(t *testing.T) {
	lines := statusLines(service.State{Installed: true, Running: true}, config.IntervalWeekly)

	if got := statusLine(t, lines, "interval:"); got != "interval: weekly" {
		t.Fatalf("unexpected interval line %q", got)
	}
}

// The failure has to come before the remedy
func TestHintedError_NoSessionBus_PutsTheRemedyAfterTheFailure(t *testing.T) {
	original := errors.New("systemctl --user daemon-reload: Failed to connect to bus: No medium found")

	got := hintedError(original).Error()

	if !strings.HasPrefix(got, original.Error()) {
		t.Fatalf("expected the failure first, got %q", got)
	}
	if !strings.Contains(got, "Do not run this with sudo") {
		t.Fatalf("expected the remedy, got %q", got)
	}
	if !errors.Is(hintedError(original), original) {
		t.Fatal("wrapping must not hide the original error")
	}
}

func TestHintedError_UnrecognisedFailure_IsUnchanged(t *testing.T) {
	original := errors.New("permission denied")

	if got := hintedError(original); got != original {
		t.Fatalf("expected the error untouched, got %q", got)
	}
}
