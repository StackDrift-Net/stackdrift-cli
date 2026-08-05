package commands

import (
	"os"
	"testing"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/detect"
)

// The probe talks to the real machine, and most of the tests that reach it never
// say anything about it, so it is answered for globally. Without this a run on
// Windows would start powershell.exe over and over for every scan test in the
// package. Same reasoning as the detect package's TestMain, which switches off
// the host web root search.
func TestMain(m *testing.M) {
	hostUpdateProbe = func(func()) (detect.UpdateStatus, bool) { return detect.UpdateStatus{}, false }
	os.Exit(m.Run())
}

// stubHostProbe replaces the machine itself, under the cache, and resets the
// cache around the test so nothing leaks into the next one.
func stubHostProbe(t *testing.T, status detect.UpdateStatus, ok bool) *int {
	t.Helper()
	asked := 0

	probe, clock := hostUpdateProbe, hostUpdateClock
	askedAt, cached := hostUpdateAskedAt, hostUpdateCached
	t.Cleanup(func() {
		hostUpdateProbe, hostUpdateClock = probe, clock
		hostUpdateAskedAt, hostUpdateCached = askedAt, cached
	})

	hostUpdateAskedAt = time.Time{}
	hostUpdateProbe = func(announce func()) (detect.UpdateStatus, bool) {
		asked++
		if announce != nil {
			announce()
		}
		return status, ok
	}
	return &asked
}

// setHostClock hands the cache a clock the test drives, since nothing here may
// wait an hour to prove the interval.
func setHostClock(now *time.Time) {
	hostUpdateClock = func() time.Time { return *now }
}

func TestHostUpdates_AnAnswer_BecomesBothCounts(t *testing.T) {
	stubHostProbe(t, detect.UpdateStatus{Pending: 12, Security: 3}, true)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	report := hostUpdates(nil)

	if report.PendingUpdates == nil || *report.PendingUpdates != 12 {
		t.Fatalf("got %v", report.PendingUpdates)
	}
	if report.SecurityUpdates == nil || *report.SecurityUpdates != 3 {
		t.Fatalf("got %v", report.SecurityUpdates)
	}
}

// The whole reason the counts are pointers. Zero would tell the website the
// machine is up to date on the strength of never having looked.
func TestHostUpdates_NobodyToAsk_LeavesBothCountsUnsent(t *testing.T) {
	stubHostProbe(t, detect.UpdateStatus{}, false)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	report := hostUpdates(nil)

	if report.PendingUpdates != nil || report.SecurityUpdates != nil {
		t.Fatalf("expected nothing sent, got %+v", report)
	}
}

// Nothing waiting is an answer, and a different one from nobody having asked.
func TestHostUpdates_NothingWaiting_SendsZeroRatherThanNothing(t *testing.T) {
	stubHostProbe(t, detect.UpdateStatus{Pending: 0, Security: 0}, true)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	report := hostUpdates(nil)

	if report.PendingUpdates == nil || *report.PendingUpdates != 0 {
		t.Fatalf("expected a reported zero, got %v", report.PendingUpdates)
	}
}

// A resident watcher comes round every ten seconds on a busy directory, and the
// number it would be asking for moves at most once a day.
func TestHostUpdates_AskedAgainStraightAway_ReusesTheAnswer(t *testing.T) {
	asked := stubHostProbe(t, detect.UpdateStatus{Pending: 4, Security: 1}, true)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	hostUpdates(nil)
	now = now.Add(10 * time.Second)
	report := hostUpdates(nil)

	if *asked != 1 {
		t.Fatalf("expected the machine asked once, got %d", *asked)
	}
	if report.PendingUpdates == nil || *report.PendingUpdates != 4 {
		t.Fatalf("expected the held answer, got %v", report.PendingUpdates)
	}
}

func TestHostUpdates_AfterTheInterval_AsksAgain(t *testing.T) {
	asked := stubHostProbe(t, detect.UpdateStatus{Pending: 4, Security: 1}, true)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	hostUpdates(nil)
	now = now.Add(hostUpdateInterval + time.Second)
	hostUpdates(nil)

	if *asked != 2 {
		t.Fatalf("expected the machine asked again after the interval, got %d", *asked)
	}
}

// A machine that cannot answer will not start being able to ten seconds later,
// so the failure is held as long as an answer would be.
func TestHostUpdates_FailedAnswer_IsHeldToo(t *testing.T) {
	asked := stubHostProbe(t, detect.UpdateStatus{}, false)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	hostUpdates(nil)
	now = now.Add(time.Minute)
	hostUpdates(nil)

	if *asked != 1 {
		t.Fatalf("expected a refusal held rather than retried, got %d", *asked)
	}
}

// Announcing a wait that is not going to happen would print a line and then
// return instantly.
func TestHostUpdates_AnsweredFromTheHeldValue_AnnouncesNoWait(t *testing.T) {
	stubHostProbe(t, detect.UpdateStatus{Pending: 4, Security: 1}, true)
	now := time.Unix(1<<30, 0)
	setHostClock(&now)

	hostUpdates(nil)
	announced := false
	now = now.Add(time.Minute)
	hostUpdates(func() { announced = true })

	if announced {
		t.Fatal("expected no wait announced when the answer was already held")
	}
}
