package commands

import (
	"strings"
	"testing"

	"github.com/StackDrift-Net/stackdrift-cli/internal/service"
)

func foundElsewhere(t *testing.T, found []service.Installation) {
	t.Helper()

	original := installedElsewhere
	installedElsewhere = func() []service.Installation { return found }
	t.Cleanup(func() { installedElsewhere = original })
}

// The common case is a machine with one account, where nothing may be said and
// nothing may be asked.
func TestConfirmNoDoubleUp_NoOtherAccountHasOne_GoesAhead(t *testing.T) {
	foundElsewhere(t, nil)

	if !confirmNoDoubleUp() {
		t.Fatal("a clean machine must install without a question")
	}
}

// go test gives the process a stdin that is not a terminal, which is the shape
// a scripted install has. It must not stop on a question nobody can answer.
func TestConfirmNoDoubleUp_AnotherAccountHasOneAndNobodyToAsk_GoesAhead(t *testing.T) {
	foundElsewhere(t, []service.Installation{{Account: "ubuntu", Detail: "/home/ubuntu/x"}})

	if !confirmNoDoubleUp() {
		t.Fatal("an unattended install must not be blocked by a prompt")
	}
}

func TestCurrentAccount_Always_NamesSomebody(t *testing.T) {
	if currentAccount() == "" {
		t.Fatal("the warning names the account, so it can never be blank")
	}
}

func countingElsewhere(t *testing.T) *int {
	t.Helper()

	calls := 0
	original := installedElsewhere
	installedElsewhere = func() []service.Installation {
		calls++
		return nil
	}
	t.Cleanup(func() { installedElsewhere = original })
	return &calls
}

// The check is only worth anything if the install paths actually run it, and
// both of them are reachable without a terminal.
func TestServiceInstall_Always_LooksForAnotherAccountsWatcher(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	calls := countingElsewhere(t)

	original := installPlan
	installPlan = func(service.Plan) error { return nil }
	t.Cleanup(func() { installPlan = original })

	if err := serviceInstall([]string{"install", "--interval", "daily"}); err != nil {
		t.Fatal(err)
	}

	if *calls != 1 {
		t.Fatalf("expected the install to check once, got %d", *calls)
	}
}

func TestOfferWatchService_Always_LooksForAnotherAccountsWatcher(t *testing.T) {
	t.Setenv("STACKDRIFT_HOME", t.TempDir())
	// A machine that already has a service is answered by adopting it, so the
	// home has to be one with nothing installed or this depends on whether the
	// machine running the tests happens to have a watcher.
	t.Setenv("HOME", t.TempDir())
	calls := countingElsewhere(t)

	// Nobody is at a terminal, so the offer takes its default and then stops at
	// the interval question. What matters is that the check ran before either.
	offerWatchService(false)

	if *calls != 1 {
		t.Fatalf("expected the offer to check once, got %d", *calls)
	}
}

// A machine with one account is the common case, and it must not be asked a
// question about a second watcher that is not there.
func TestConfirmNoDoubleUp_NoOtherAccountHasOne_SaysNothing(t *testing.T) {
	foundElsewhere(t, nil)

	if out := captureOutput(func() { confirmNoDoubleUp() }); out != "" {
		t.Fatalf("expected silence, got %q", out)
	}
}

func TestConfirmNoDoubleUp_AnotherAccountHasOne_NamesIt(t *testing.T) {
	foundElsewhere(t, []service.Installation{{Account: "ubuntu", Detail: "/home/ubuntu/x"}})

	out := captureOutput(func() { confirmNoDoubleUp() })
	if !strings.Contains(out, "ubuntu") {
		t.Fatalf("expected the other account named, got %q", out)
	}
}
