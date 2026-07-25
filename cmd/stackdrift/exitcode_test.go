package main

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/digitalaffinity-au/stackdrift-cli/internal/api"
)

func lockoutRefusal() error {
	return &api.Error{
		Status:  http.StatusPaymentRequired,
		Message: "Reactivate your plan to make changes.",
		Reason:  api.ReasonSubscriptionRequired,
	}
}

func runWith(t *testing.T, err error) (int, string) {
	t.Helper()
	var calls []string
	var stdout, stderr bytes.Buffer

	code := run([]string{"scan"}, stubRegistry(&calls, err), &stdout, &stderr)

	return code, stderr.String()
}

// The point of the whole change: check exits 1 when it finds an advisory, so a
// lapsed plan sharing that code would turn a pipeline red for a billing reason
// and have it triaged as a security finding.
func TestRun_LapsedPlan_ExitsWithACodeOfItsOwn(t *testing.T) {
	code, _ := runWith(t, lockoutRefusal())

	if code == exitFailure {
		t.Fatal("a lapsed plan must not share the exit code that reports a CVE")
	}
	if code != exitPlanLapsed {
		t.Fatalf("expected exit %d, got %d", exitPlanLapsed, code)
	}
}

func TestRun_LapsedPlan_ExplainsWhatWasRefused(t *testing.T) {
	_, stderr := runWith(t, lockoutRefusal())

	if !strings.Contains(stderr, "Reactivate your plan to make changes.") {
		t.Fatalf("expected the server's reason on stderr, got %q", stderr)
	}
}

func TestRun_LapsedPlan_PointsAtTheBillingPage(t *testing.T) {
	t.Setenv("STACKDRIFT_URL", "https://example.test")

	_, stderr := runWith(t, lockoutRefusal())

	if !strings.Contains(stderr, "https://example.test/billing") {
		t.Fatalf("expected the billing page named, got %q", stderr)
	}
}

// Reads and deletes are deliberately still allowed, so saying "everything is
// refused" would send someone to support instead of to the escape hatch.
func TestRun_LapsedPlan_SaysWhatStillWorks(t *testing.T) {
	_, stderr := runWith(t, lockoutRefusal())

	if !strings.Contains(stderr, "Reading and removing still work") {
		t.Fatalf("expected the escape hatch explained, got %q", stderr)
	}
}

// A stale build is refused with 426 before anything else is judged, so being
// locked out can never stop it replacing itself. The marker stands in for the
// re-run, which is what makes a second refusal final.
func TestRun_UpgradeRefusal_IsNotReadAsALapsedPlan(t *testing.T) {
	t.Setenv(upgradedMarker, "1")

	code, stderr := runWith(t, &api.Error{Status: http.StatusUpgradeRequired, Message: "CLI update required"})

	if code == exitPlanLapsed {
		t.Fatal("an upgrade refusal must not be reported as a lapsed plan")
	}
	if code != exitFailure {
		t.Fatalf("expected exit %d, got %d", exitFailure, code)
	}
	if strings.Contains(stderr, "/billing") {
		t.Fatalf("expected no billing hint for an upgrade refusal, got %q", stderr)
	}
}

func TestRun_CveFound_StillExitsWithTheFailureCode(t *testing.T) {
	code, _ := runWith(t, errors.New("2 technology CVEs and 0 dependency CVEs found"))

	if code != exitFailure {
		t.Fatalf("the CI gate must stay on exit %d, got %d", exitFailure, code)
	}
}

// 402 is also how the server answers other billing refusals, and only the
// lapsed plan is the one this code means.
func TestRun_DifferentBillingRefusal_DoesNotClaimTheLapsedPlanCode(t *testing.T) {
	code, _ := runWith(t, &api.Error{
		Status:  http.StatusPaymentRequired,
		Message: "Confirm the change first.",
		Reason:  "plan_change_not_confirmed",
	})

	if code != exitFailure {
		t.Fatalf("expected exit %d, got %d", exitFailure, code)
	}
}

func TestRun_Success_ExitsZero(t *testing.T) {
	code, _ := runWith(t, nil)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}
